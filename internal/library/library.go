// Package library holds Tabernacle's use cases: the operations that span the
// collection on disk and the index that describes it.
//
// It exists so that "save this song" lives in one place. That operation is
// transactional across two subsystems — validate, write the file atomically,
// then update the index and the search rows — and putting it in HTTP handlers
// would duplicate it across create, edit, import and delete, while putting it
// in main would make it untestable without starting a server.
//
// One rule runs through everything here: the file is written first and the
// index second. A file on disk with a stale index entry is repaired by the next
// scan. The other order loses the song.
package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/songfs"
)

// ErrSyncBusy reports that a scan is already in progress.
var ErrSyncBusy = errors.New("a scan is already running")

// ErrNotFound reports that no song answers to the given slug.
var ErrNotFound = errors.New("song not found")

// ErrMoved reports that a slug has been superseded. NewSlug carries where the
// caller should look instead, so the HTTP layer can redirect rather than
// breaking a bookmark.
type ErrMoved struct{ NewSlug string }

func (e ErrMoved) Error() string { return "song moved to " + e.NewSlug }

// Library is the application's view of the collection.
type Library struct {
	fs  *songfs.Store
	idx *index.Index
	log *slog.Logger

	// syncing serialises scans. The startup scan, the periodic ticker and the
	// reindex button can otherwise overlap and fight over the same rows; a
	// second caller is told a scan is running rather than being queued.
	syncing sync.Mutex

	now func() time.Time

	// lastScan is what the most recent scan did, so the interface can say
	// something more useful than "done". A button that reports nothing is
	// indistinguishable from a button that does nothing.
	lastMu   sync.Mutex
	lastScan SyncReport
	lastAt   time.Time
}

// LastScan returns the most recent scan's result and when it happened.
func (l *Library) LastScan() (SyncReport, time.Time) {
	l.lastMu.Lock()
	defer l.lastMu.Unlock()
	return l.lastScan, l.lastAt
}

// New assembles the library. now is injectable so tests do not depend on the
// clock.
func New(store *songfs.Store, idx *index.Index, log *slog.Logger) *Library {
	if log == nil {
		log = slog.Default()
	}
	return &Library{fs: store, idx: idx, log: log, now: time.Now}
}

// SyncReport says what a scan did.
type SyncReport struct {
	Scanned   int
	Added     int
	Updated   int
	Renamed   int
	Removed   int
	Unchanged int
	Failed    int
	Duration  time.Duration
}

// Sync reconciles the index with the collection on disk.
//
// Changes are detected by hashing every file rather than by comparing size and
// modification time. Those are not sufficient: changing [G] to [C] leaves the
// size identical, and an edit within one clock tick leaves the modification
// time identical, which would silently leave the index stale. Hashing a
// thousand small files costs a few milliseconds, and it removes the whole class
// of "why does search not find my edit" bug.
func (l *Library) Sync(ctx context.Context) (SyncReport, error) {
	if !l.syncing.TryLock() {
		return SyncReport{}, ErrSyncBusy
	}
	defer l.syncing.Unlock()

	started := l.now()
	var rep SyncReport

	// Slugs must be resolved against every path at once, because breaking a
	// collision deterministically means knowing which paths collide. This walk
	// reads no files.
	paths, err := l.fs.Paths()
	if err != nil {
		return rep, fmt.Errorf("library: list the collection: %w", err)
	}
	slugs := index.ResolveSlugs(paths)
	onDisk := make(map[string]bool, len(paths))
	for _, p := range paths {
		onDisk[p] = true
	}

	stamps, err := l.idx.Stamps(ctx)
	if err != nil {
		return rep, err
	}

	seen := make(map[string]bool, len(paths))
	// Paths whose row was carried over to a new location. They are still present
	// in stamps but must not be deleted afterwards, or a rename would be
	// reported, and counted, as a delete plus an add.
	movedFrom := make(map[string]bool)

	err = l.fs.Scan(func(e songfs.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rep.Scanned++
		seen[e.RelPath] = true

		slug, ok := slugs[e.RelPath]
		if !ok {
			// The file appeared between the two walks. It will be picked up by
			// the next scan; inventing a slug now could collide.
			return nil
		}

		stamp, known := stamps[e.RelPath]
		if known && stamp.Hash == e.Hash && stamp.Slug == slug {
			rep.Unchanged++
			return nil
		}

		renamed := false
		if !known {
			from, moved, err := l.detectRename(ctx, e, slug, onDisk)
			if err != nil {
				return err
			}
			if moved {
				movedFrom[from] = true
				renamed = true
				rep.Renamed++
			}
		}

		if e.Err != nil {
			if err := l.indexBroken(ctx, e, slug); err != nil {
				return err
			}
			rep.Failed++
		} else if _, err := l.idx.Upsert(ctx, recordFor(e, slug)); err != nil {
			return err
		}

		// A rename is already accounted for; counting it again as an addition
		// would make the report describe two events where one happened.
		switch {
		case renamed:
		case known:
			rep.Updated++
		default:
			rep.Added++
		}
		return nil
	})
	if err != nil {
		return rep, fmt.Errorf("library: scan the collection: %w", err)
	}

	for rel := range stamps {
		if seen[rel] || movedFrom[rel] {
			continue
		}
		if err := l.idx.DeleteByRelPath(ctx, rel); err != nil {
			return rep, err
		}
		rep.Removed++
	}

	l.idx.FullScanDone()
	rep.Duration = l.now().Sub(started)

	l.lastMu.Lock()
	l.lastScan, l.lastAt = rep, l.now()
	l.lastMu.Unlock()

	l.log.Info("scanned the collection",
		"scanned", rep.Scanned, "added", rep.Added, "updated", rep.Updated,
		"renamed", rep.Renamed, "removed", rep.Removed, "unchanged", rep.Unchanged,
		"failed", rep.Failed, "took", rep.Duration)
	return rep, nil
}

// detectRename recognises a file that moved rather than one that is new.
//
// A song whose content matches an indexed row whose own path has disappeared has
// been moved, so the existing row is updated in place. Keeping the row keeps the
// song's saved transposition and its play count, which would otherwise be
// silently discarded by dragging a file into a different artist folder.
//
// The check that the old path is really gone is what stops a duplicated file
// from being mistaken for a move.
func (l *Library) detectRename(ctx context.Context, e songfs.Entry, slug string, onDisk map[string]bool) (string, bool, error) {
	if e.Hash == "" {
		return "", false, nil
	}
	old, found, err := l.idx.ByHash(ctx, e.Hash)
	if err != nil {
		return "", false, err
	}
	if !found || old.RelPath == e.RelPath || onDisk[old.RelPath] {
		return "", false, nil
	}
	if err := l.idx.UpdatePath(ctx, old.ID, e.RelPath, slug); err != nil {
		return "", false, err
	}
	if err := l.idx.MoveState(ctx, old.Slug, slug); err != nil {
		return "", false, err
	}
	l.log.Info("song moved", "from", old.RelPath, "to", e.RelPath)
	return old.RelPath, true, nil
}

// indexBroken records a file that will not parse, keeping whatever was last read
// from it so the song stays findable and repairable.
func (l *Library) indexBroken(ctx context.Context, e songfs.Entry, slug string) error {
	msg, line := describeParseError(e.Err)
	updated, err := l.idx.MarkBroken(ctx, e.RelPath, e.Hash, e.Size, e.ModTime, msg, line)
	if err != nil {
		return err
	}
	if updated {
		l.log.Warn("song no longer parses", "path", e.RelPath, "line", line, "error", msg)
		return nil
	}

	// Never seen before and already broken: index it under its filename so it
	// is at least visible and can be opened in the editor.
	r := index.Record{
		RelPath:        e.RelPath,
		Slug:           slug,
		Title:          titleFromPath(e.RelPath),
		Artist:         artistFromPath(e.RelPath),
		Hash:           e.Hash,
		Size:           e.Size,
		ModTime:        e.ModTime,
		ParseError:     msg,
		ParseErrorLine: line,
	}
	if _, err := l.idx.Upsert(ctx, r); err != nil {
		return err
	}
	l.log.Warn("song does not parse", "path", e.RelPath, "line", line, "error", msg)
	return nil
}

// describeParseError reduces an error to a message and a line number.
func describeParseError(err error) (string, int) {
	if err == nil {
		return "", 0
	}
	var se *song.SyntaxError
	if errors.As(err, &se) {
		return se.Msg, se.Line
	}
	return err.Error(), 0
}

// recordFor builds an index record from a parsed file.
func recordFor(e songfs.Entry, slug string) index.Record {
	chords := e.Song.Chords()
	symbols := make([]string, 0, len(chords))
	for _, c := range chords {
		symbols = append(symbols, c.String())
	}
	return index.Record{
		RelPath:  e.RelPath,
		Slug:     slug,
		Title:    e.Song.Title,
		Artist:   e.Song.Artist,
		Key:      e.Song.Key,
		Capo:     e.Song.Capo,
		Tuning:   e.Song.Tuning,
		Notes:    e.Song.Notes,
		Genre:    e.Song.Genre,
		Chords:   symbols,
		Lyrics:   e.Song.PlainText(),
		Sections: len(e.Song.Sections),
		Lines:    e.Song.CountLines(),
		Size:     e.Size,
		ModTime:  e.ModTime,
		Hash:     e.Hash,
	}
}

// titleFromPath guesses a title for a file that cannot be parsed.
func titleFromPath(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}

// artistFromPath guesses an artist from the directory a file sits in.
func artistFromPath(rel string) string {
	if dir := path.Dir(rel); dir != "." && dir != "/" {
		return path.Base(dir)
	}
	return ""
}
