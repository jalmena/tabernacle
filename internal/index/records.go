package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Record is one indexed song.
//
// The same type is used for writing and reading. A caller building a record from
// a file sets everything except ID, FirstSeen and IndexedAt, which the index
// owns; State is filled in only by the read paths that ask for it.
type Record struct {
	ID      int64
	RelPath string
	Slug    string

	Title  string
	Artist string
	Key    string
	Capo   int
	Tuning string
	Notes  string
	Genre  string
	Chords []string // canonical English spelling, in order of first appearance

	// Lyrics is the song's text with chord brackets removed, used only to feed
	// full-text search. It is not stored as a column and is never read back.
	Lyrics string

	Sections int
	Lines    int

	Size    int64
	ModTime time.Time
	Hash    string

	ParseError     string
	ParseErrorLine int

	FirstSeen time.Time
	IndexedAt time.Time
}

// Broken reports whether the file failed to parse.
func (r Record) Broken() bool { return r.ParseError != "" }

// Stamp is the minimum needed to decide whether a file has changed.
type Stamp struct {
	ID      int64
	Slug    string
	Hash    string
	Size    int64
	ModTime time.Time
}

// Stamps returns one entry per indexed song, keyed by path.
//
// A scan compares against this to decide what to insert, update and delete.
func (x *Index) Stamps(ctx context.Context) (map[string]Stamp, error) {
	rows, err := x.db.QueryContext(ctx,
		`SELECT id, rel_path, slug, content_hash, size_bytes, mtime_unix_ns FROM songs`)
	if err != nil {
		return nil, fmt.Errorf("index: list stamps: %w", err)
	}
	defer rows.Close()

	out := make(map[string]Stamp)
	for rows.Next() {
		var (
			rel   string
			s     Stamp
			mtime int64
		)
		if err := rows.Scan(&s.ID, &rel, &s.Slug, &s.Hash, &s.Size, &mtime); err != nil {
			return nil, err
		}
		s.ModTime = time.Unix(0, mtime)
		out[rel] = s
	}
	return out, rows.Err()
}

// Upsert writes one song and everything derived from it in a single
// transaction, so the index never holds a song whose chords or search rows are
// half updated.
func (x *Index) Upsert(ctx context.Context, r Record) (int64, error) {
	if r.RelPath == "" || r.Slug == "" {
		return 0, errors.New("index: a record needs a path and a slug")
	}
	now := time.Now()
	if r.IndexedAt.IsZero() {
		r.IndexedAt = now
	}
	if r.FirstSeen.IsZero() {
		r.FirstSeen = now
	}

	var id int64
	err := x.inTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`INSERT INTO songs (
			     rel_path, slug, title, artist, song_key, capo, tuning, genre, genre_fold, notes,
			     sort_title, sort_artist, section_count, line_count,
			     size_bytes, mtime_unix_ns, content_hash,
			     parse_error, parse_error_line, first_seen_at, indexed_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(rel_path) DO UPDATE SET
			     slug             = excluded.slug,
			     title            = excluded.title,
			     artist           = excluded.artist,
			     song_key         = excluded.song_key,
			     capo             = excluded.capo,
			     tuning           = excluded.tuning,
			     genre            = excluded.genre,
			     genre_fold       = excluded.genre_fold,
			     notes            = excluded.notes,
			     sort_title       = excluded.sort_title,
			     sort_artist      = excluded.sort_artist,
			     section_count    = excluded.section_count,
			     line_count       = excluded.line_count,
			     size_bytes       = excluded.size_bytes,
			     mtime_unix_ns    = excluded.mtime_unix_ns,
			     content_hash     = excluded.content_hash,
			     parse_error      = excluded.parse_error,
			     parse_error_line = excluded.parse_error_line,
			     indexed_at       = excluded.indexed_at
			 RETURNING id`,
			r.RelPath, r.Slug, r.Title, r.Artist, r.Key, r.Capo, r.Tuning, r.Genre, Slugify(r.Genre), r.Notes,
			sortKey(r.Title), sortKey(r.Artist), r.Sections, r.Lines,
			r.Size, r.ModTime.UnixNano(), r.Hash,
			r.ParseError, r.ParseErrorLine, r.FirstSeen.UnixNano(), r.IndexedAt.UnixNano(),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("index: upsert %s: %w", r.RelPath, err)
		}

		if err := replaceChords(ctx, tx, id, r.Chords); err != nil {
			return err
		}
		if err := replaceFTS(ctx, tx, id, r); err != nil {
			return err
		}
		// The slug exists again, so any redirect that pointed away from it is
		// stale: a song restored from the trash should answer on its own URL.
		return x.forgetRedirect(ctx, tx, r.Slug)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func replaceChords(ctx context.Context, tx *sql.Tx, songID int64, chords []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM song_chords WHERE song_id = ?`, songID); err != nil {
		return err
	}
	for i, sym := range chords {
		if sym == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO song_chords(song_id, symbol, ord) VALUES(?, ?, ?)`,
			songID, sym, i); err != nil {
			return err
		}
	}
	return nil
}

// replaceFTS keeps the search rowid equal to songs.id, which is what lets a
// search join straight back onto the songs table.
func replaceFTS(ctx context.Context, tx *sql.Tx, songID int64, r Record) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM song_fts WHERE rowid = ?`, songID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO song_fts(rowid, title, artist, genre, lyrics, notes, chords)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		songID, r.Title, r.Artist, r.Genre, r.Lyrics, r.Notes,
		strings.Join(r.Chords, " "))
	return err
}

// DeleteByRelPath removes a song from the index. The file itself is the
// caller's business.
func (x *Index) DeleteByRelPath(ctx context.Context, rel string) error {
	return x.inTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM songs WHERE rel_path = ?`, rel).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM song_fts WHERE rowid = ?`, id); err != nil {
			return err
		}
		// song_tags and song_chords go with it through ON DELETE CASCADE.
		_, err = tx.ExecContext(ctx, `DELETE FROM songs WHERE id = ?`, id)
		return err
	})
}

// UpdatePath records that a song moved, keeping its row and therefore its
// identity. Renames are detected by content hash during a scan.
func (x *Index) UpdatePath(ctx context.Context, id int64, relPath, slug string) error {
	_, err := x.db.ExecContext(ctx,
		`UPDATE songs SET rel_path = ?, slug = ? WHERE id = ?`, relPath, slug, id)
	if err != nil {
		return fmt.Errorf("index: move song %d to %s: %w", id, relPath, err)
	}
	return nil
}

const selectColumns = `
	s.id, s.rel_path, s.slug, s.title, s.artist, s.song_key, s.capo, s.tuning, s.genre,
	s.notes, s.section_count, s.line_count, s.size_bytes,
	s.mtime_unix_ns, s.content_hash, s.parse_error, s.parse_error_line,
	s.first_seen_at, s.indexed_at`

func scanRecord(rows interface{ Scan(...any) error }) (Record, error) {
	var (
		r                     Record
		mtime, first, indexed int64
	)
	err := rows.Scan(&r.ID, &r.RelPath, &r.Slug, &r.Title, &r.Artist, &r.Key, &r.Capo, &r.Tuning,
		&r.Genre, &r.Notes, &r.Sections, &r.Lines, &r.Size,
		&mtime, &r.Hash, &r.ParseError, &r.ParseErrorLine, &first, &indexed)
	if err != nil {
		return Record{}, err
	}
	r.ModTime = time.Unix(0, mtime)
	r.FirstSeen = time.Unix(0, first)
	r.IndexedAt = time.Unix(0, indexed)
	return r, nil
}

// BySlug returns one song by its URL identifier.
func (x *Index) BySlug(ctx context.Context, slug string) (Record, bool, error) {
	row := x.db.QueryRowContext(ctx,
		`SELECT`+selectColumns+` FROM songs s WHERE s.slug = ?`, slug)
	r, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("index: read song %q: %w", slug, err)
	}
	if err := x.loadDetails(ctx, []*Record{&r}); err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

// ByHash finds a song by its content, which is how a rename is recognised.
func (x *Index) ByHash(ctx context.Context, hash string) (Record, bool, error) {
	row := x.db.QueryRowContext(ctx,
		`SELECT`+selectColumns+` FROM songs s WHERE s.content_hash = ? LIMIT 1`, hash)
	r, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("index: read song by hash: %w", err)
	}
	return r, true, nil
}

// ByTitle finds the songs with a title, compared the way the listing sorts
// them: regardless of case, accents and a leading article.
func (x *Index) ByTitle(ctx context.Context, title string) ([]Record, error) {
	rows, err := x.db.QueryContext(ctx,
		`SELECT`+selectColumns+` FROM songs s WHERE s.sort_title = ? ORDER BY s.sort_artist`, sortKey(title))
	if err != nil {
		return nil, fmt.Errorf("index: find songs by title: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("index: find songs by title: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadDetails fills in the chords of already-scanned records with one query
// rather than one per song.
func (x *Index) loadDetails(ctx context.Context, recs []*Record) error {
	if len(recs) == 0 {
		return nil
	}
	byID := make(map[int64]*Record, len(recs))
	ids := make([]any, 0, len(recs))
	for _, r := range recs {
		byID[r.ID] = r
		ids = append(ids, r.ID)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	chordRows, err := x.db.QueryContext(ctx,
		`SELECT song_id, symbol FROM song_chords
		 WHERE song_id IN (`+placeholders+`) ORDER BY song_id, ord`, ids...)
	if err != nil {
		return fmt.Errorf("index: load chords: %w", err)
	}
	defer chordRows.Close()
	for chordRows.Next() {
		var id int64
		var sym string
		if err := chordRows.Scan(&id, &sym); err != nil {
			return err
		}
		if r := byID[id]; r != nil {
			r.Chords = append(r.Chords, sym)
		}
	}
	return chordRows.Err()
}

// Stats summarises the collection, for the settings page and the health check.
type Stats struct {
	Songs   int
	Artists int
	Genres  int
	Broken  int
}

// Stats counts what is in the index.
func (x *Index) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := x.db.QueryRowContext(ctx,
		`SELECT
		   (SELECT COUNT(*) FROM songs),
		   (SELECT COUNT(DISTINCT artist) FROM songs WHERE artist <> ''),
		   (SELECT COUNT(DISTINCT genre_fold) FROM songs WHERE genre <> ''),
		   (SELECT COUNT(*) FROM songs WHERE parse_error <> '')`,
	).Scan(&s.Songs, &s.Artists, &s.Genres, &s.Broken)
	if err != nil {
		return Stats{}, fmt.Errorf("index: read stats: %w", err)
	}
	return s, nil
}

// timeFromNanos converts a stored timestamp, mapping zero to the zero Time
// rather than to 1970 so that "never" is distinguishable from "the epoch".
func timeFromNanos(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// MarkBroken records that a file no longer parses, without discarding what was
// last successfully read from it.
//
// Only the file's identity and the error are touched, so the title, artist,
// genre and search rows from the last good version stay in place. That is what
// lets a song with a typo in it remain findable, and lets the interface say
// which line to fix, instead of the song appearing to have been lost.
//
// It reports false when there is no existing row, in which case the caller
// should insert one so that the file is at least visible.
func (x *Index) MarkBroken(ctx context.Context, rel, hash string, size int64, modTime time.Time, msg string, line int) (bool, error) {
	res, err := x.db.ExecContext(ctx,
		`UPDATE songs SET
		     content_hash     = ?,
		     size_bytes       = ?,
		     mtime_unix_ns    = ?,
		     parse_error      = ?,
		     parse_error_line = ?,
		     indexed_at       = ?
		 WHERE rel_path = ?`,
		hash, size, modTime.UnixNano(), msg, line, time.Now().UnixNano(), rel)
	if err != nil {
		return false, fmt.Errorf("index: mark %s broken: %w", rel, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
