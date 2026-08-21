package index

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// The markers snippet() wraps a match in.
//
// They are control characters that cannot occur in a parsed .song file, and
// they exist to keep search results out of the cross-site-scripting business.
// Asking SQLite for "<mark>" and "</mark>" directly would return a string
// mixing our markup with the stored lyric, leaving a choice between escaping
// the whole thing and losing the highlight, or emitting it raw and shipping an
// injection. Splitting on sentinels instead means every piece of user text is
// escaped by the template, and the <mark> lives in the template where it
// belongs.
const (
	hitOpen  = "\x02"
	hitClose = "\x03"
)

// Search input limits. A query longer than this is a mistake or an attack, and
// truncating beats building a thousand-term FTS expression.
const (
	maxTerms     = 16
	maxTermRunes = 64
)

// SortOrder selects how results are ordered.
type SortOrder string

const (
	SortRelevance SortOrder = "relevance"
	SortTitle     SortOrder = "title"
	SortArtist    SortOrder = "artist"
	SortRecent    SortOrder = "recent"
	SortAdded     SortOrder = "added"
)

// Query describes a library listing.
//
// Text is free text only: the field-scoped parts of what the user typed have
// already been lifted out by ParseQuery into the fields below, so nothing the
// user writes is ever handed to the full-text parser as syntax.
type Query struct {
	Text   string
	Genre  string
	Artist string
	Key    string
	Chord  string
	Broken bool
	Sort   SortOrder
	Limit  int
	Offset int
}

// Frag is a piece of a search excerpt. Hit marks the part that matched.
type Frag struct {
	Text string
	Hit  bool
}

// Hit is one search result.
//
// There is no score field: the ordering is done by SQL, and carrying bm25 back
// into Go only to discard it was a number nobody read.
type Hit struct {
	Record
	Frags []Frag // excerpt from the lyrics, empty when the query was not free text
}

// ParseQuery turns what the user typed into a Query, lifting out field-scoped
// terms such as "tag:hymn" or "chord:F#m" so they become ordinary SQL
// conditions rather than full-text syntax.
func ParseQuery(raw string) Query {
	q := Query{Sort: SortRelevance}
	var free []string
	for _, field := range strings.Fields(raw) {
		name, value, ok := strings.Cut(field, ":")
		if !ok || value == "" {
			free = append(free, field)
			continue
		}
		switch strings.ToLower(name) {
		case "genre", "genero", "género":
			q.Genre = value
		case "artist", "artista":
			q.Artist = value
		case "key", "tono":
			q.Key = value
		case "chord", "acorde":
			q.Chord = value
		default:
			free = append(free, field)
		}
	}
	q.Text = strings.Join(free, " ")
	if q.Text == "" {
		q.Sort = SortTitle // relevance is meaningless without a search term
	}
	return q
}

// ftsQuery builds a safe FTS5 MATCH expression from free text.
//
// Every term is quoted, with any embedded quote doubled, and given a prefix
// star. That means an input of `NEAR(a b, 2)` or `" OR 1=1 --` is a search for
// those characters rather than an expression the full-text parser tries to
// interpret, which is the single most common way an FTS5 application turns a
// search box into a 500.
//
// It returns "" when there is nothing to search for.
func ftsQuery(text string) string {
	terms := make([]string, 0, maxTerms)
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		// Split on anything that is not a letter or a digit, which matches how
		// the unicode61 tokenizer split the text on the way in. An apostrophe is
		// a separator on both sides, so "ain't" is indexed and searched as the
		// two terms "ain" and "t"; searching "ain" or "ain't" finds the song,
		// while searching "aint" does not, because that is a different token.
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		runes := []rune(word)
		if len(runes) > maxTermRunes {
			runes = runes[:maxTermRunes]
		}
		term := string(runes)
		terms = append(terms, `"`+strings.ReplaceAll(term, `"`, `""`)+`"*`)
		if len(terms) == maxTerms {
			break
		}
	}
	return strings.Join(terms, " AND ")
}

// splitFrags turns a sentinel-marked snippet into fragments.
func splitFrags(s string) []Frag {
	var out []Frag
	for s != "" {
		i := strings.Index(s, hitOpen)
		if i < 0 {
			return append(out, Frag{Text: s})
		}
		if i > 0 {
			out = append(out, Frag{Text: s[:i]})
		}
		s = s[i+len(hitOpen):]
		j := strings.Index(s, hitClose)
		if j < 0 {
			return append(out, Frag{Text: s, Hit: true})
		}
		out = append(out, Frag{Text: s[:j], Hit: true})
		s = s[j+len(hitClose):]
	}
	return out
}

// List returns a page of songs and the total number that matched.
func (x *Index) List(ctx context.Context, q Query) ([]Hit, int, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 200 {
		q.Limit = 200
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Sort == "" {
		// An unset order means "the most useful one": relevance when there is
		// something to rank by, alphabetical when there is not.
		if q.Text != "" {
			q.Sort = SortRelevance
		} else {
			q.Sort = SortTitle
		}
	}

	match := ftsQuery(q.Text)
	var (
		joins  []string
		wheres []string
		args   []any
	)

	if match != "" {
		joins = append(joins, `JOIN song_fts f ON f.rowid = s.id`)
		wheres = append(wheres, `song_fts MATCH ?`)
		args = append(args, match)
	}
	if q.Genre != "" {
		wheres = append(wheres, `s.genre_fold = ?`)
		args = append(args, Slugify(q.Genre))
	}
	if q.Artist != "" {
		wheres = append(wheres, `s.sort_artist = ?`)
		args = append(args, sortKey(q.Artist))
	}
	if q.Key != "" {
		wheres = append(wheres, `s.song_key = ?`)
		args = append(args, q.Key)
	}
	if q.Chord != "" {
		wheres = append(wheres, `EXISTS (SELECT 1 FROM song_chords sc
		                                 WHERE sc.song_id = s.id AND sc.symbol = ?)`)
		args = append(args, q.Chord)
	}
	if q.Broken {
		wheres = append(wheres, `s.parse_error <> ''`)
	}

	where := ""
	if len(wheres) > 0 {
		where = " WHERE " + strings.Join(wheres, " AND ")
	}
	join := strings.Join(joins, " ")

	var total int
	if err := x.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM songs s `+join+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("index: count matches: %w", err)
	}

	// Recency needs the durable state table, which is deliberately not joined by
	// a foreign key; a left join on slug keeps songs with no state visible.
	stateJoin := ""
	orderBy := "s.sort_title, s.sort_artist"
	switch q.Sort {
	case SortRelevance:
		if match != "" {
			orderBy = "score, s.sort_title"
		}
	case SortArtist:
		orderBy = "s.sort_artist, s.sort_title"
	case SortRecent:
		stateJoin = " LEFT JOIN song_state ss ON ss.slug = s.slug"
		orderBy = "ss.last_viewed_at IS NULL, ss.last_viewed_at DESC, s.sort_title"
	case SortAdded:
		orderBy = "s.first_seen_at DESC, s.sort_title"
	}

	scoreExpr, snippetExpr := "0", "''"
	if match != "" {
		scoreExpr = "bm25(song_fts, 12.0, 8.0, 4.0, 1.0, 2.0, 3.0)"
		// Column 3 is lyrics. The markers are parameters, never concatenated.
		snippetExpr = "snippet(song_fts, 3, ?, ?, '…', 12)"
	}

	query := `SELECT` + selectColumns + `, ` + scoreExpr + ` AS score, ` + snippetExpr + ` AS frag
	          FROM songs s ` + join + stateJoin + where +
		` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`

	// Argument order must match the placeholders: the snippet markers sit in the
	// select list, so they come before the where-clause arguments.
	full := make([]any, 0, len(args)+4)
	if match != "" {
		full = append(full, hitOpen, hitClose)
	}
	full = append(full, args...)
	full = append(full, q.Limit, q.Offset)

	rows, err := x.db.QueryContext(ctx, query, full...)
	if err != nil {
		return nil, 0, fmt.Errorf("index: search: %w", err)
	}
	defer rows.Close()

	var (
		hits []Hit
		ptrs []*Record
	)
	for rows.Next() {
		var (
			r                     Record
			score                 float64
			frag                  string
			mtime, first, indexed int64
		)
		if err := rows.Scan(&r.ID, &r.RelPath, &r.Slug, &r.Title, &r.Artist, &r.Key, &r.Capo,
			&r.Tuning, &r.Genre, &r.Notes, &r.Sections, &r.Lines, &r.Size,
			&mtime, &r.Hash, &r.ParseError, &r.ParseErrorLine, &first, &indexed,
			&score, &frag); err != nil {
			return nil, 0, err
		}
		r.ModTime = timeFromNanos(mtime)
		r.FirstSeen = timeFromNanos(first)
		r.IndexedAt = timeFromNanos(indexed)
		// score is selected because ORDER BY refers to it; it is not kept.
		_ = score
		hits = append(hits, Hit{Record: r, Frags: splitFrags(frag)})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range hits {
		ptrs = append(ptrs, &hits[i].Record)
	}
	if err := x.loadDetails(ctx, ptrs); err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

// GenreCount is a genre and how many songs carry it.
type GenreCount struct {
	Name  string
	Fold  string
	Count int
}

// Genres lists the genres in use, most used first.
func (x *Index) Genres(ctx context.Context) ([]GenreCount, error) {
	rows, err := x.db.QueryContext(ctx,
		`SELECT genre, genre_fold, COUNT(*) AS n FROM songs
		 WHERE genre <> '' GROUP BY genre_fold ORDER BY n DESC, genre_fold`)
	if err != nil {
		return nil, fmt.Errorf("index: list genres: %w", err)
	}
	defer rows.Close()
	var out []GenreCount
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Name, &g.Fold, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ArtistCount is an artist and how many of their songs are in the collection.
type ArtistCount struct {
	Name  string
	Count int
}

// Artists lists the artists in the collection alphabetically.
func (x *Index) Artists(ctx context.Context) ([]ArtistCount, error) {
	rows, err := x.db.QueryContext(ctx,
		`SELECT artist, COUNT(*) FROM songs WHERE artist <> ''
		 GROUP BY artist ORDER BY sort_artist`)
	if err != nil {
		return nil, fmt.Errorf("index: list artists: %w", err)
	}
	defer rows.Close()
	var out []ArtistCount
	for rows.Next() {
		var a ArtistCount
		if err := rows.Scan(&a.Name, &a.Count); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
