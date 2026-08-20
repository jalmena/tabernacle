package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/song"
)

// SettingNotation is the key under which the default note names are stored.
const SettingNotation = "notation"

// SongView is everything needed to show one song.
//
// Song comes from the file on disk, never from the index. The index knows how to
// find a song and how to search for one; it is not where a song lives, and
// reading content from it would quietly make the database authoritative.
type SongView struct {
	Record index.Record
	Song   song.Song
	Hash   string
	State  index.SongState

	// ParseError is set when the file currently on disk does not parse. Record
	// then still holds the last version that did, so the song remains findable
	// while the interface offers to fix it.
	ParseError error
}

// Get loads one song by slug.
//
// A slug that has been superseded returns ErrMoved rather than ErrNotFound, so
// that renaming a file redirects a bookmark instead of breaking it.
func (l *Library) Get(ctx context.Context, slug string) (SongView, error) {
	rec, found, err := l.idx.BySlug(ctx, slug)
	if err != nil {
		return SongView{}, err
	}
	if !found {
		to, moved, err := l.idx.Redirect(ctx, slug)
		if err != nil {
			return SongView{}, err
		}
		if moved && to != slug {
			return SongView{}, ErrMoved{NewSlug: to}
		}
		return SongView{}, ErrNotFound
	}

	entry, err := l.fs.Read(rec.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// The index is ahead of the disk: the file was removed outside the
			// application and the next scan will drop the row.
			return SongView{}, ErrNotFound
		}
		return SongView{}, fmt.Errorf("library: read %s: %w", rec.RelPath, err)
	}

	state, err := l.idx.State(ctx, rec.Slug)
	if err != nil {
		return SongView{}, err
	}
	return SongView{
		Record:     rec,
		Song:       entry.Song,
		Hash:       entry.Hash,
		State:      state,
		ParseError: entry.Err,
	}, nil
}

// RawFile returns a song's bytes as they are on disk, for download and for the
// editor.
//
// The editor needs the bytes even when they do not parse: a broken file is
// exactly the file somebody is trying to repair.
func (l *Library) RawFile(ctx context.Context, slug string) (data []byte, hash, filename string, err error) {
	rec, found, err := l.idx.BySlug(ctx, slug)
	if err != nil {
		return nil, "", "", err
	}
	if !found {
		to, moved, err := l.idx.Redirect(ctx, slug)
		if err != nil {
			return nil, "", "", err
		}
		if moved && to != slug {
			return nil, "", "", ErrMoved{NewSlug: to}
		}
		return nil, "", "", ErrNotFound
	}
	data, hash, err = l.fs.ReadRaw(rec.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", "", ErrNotFound
		}
		return nil, "", "", fmt.Errorf("library: read %s: %w", rec.RelPath, err)
	}
	return data, hash, path.Base(rec.RelPath), nil
}

// BrowseResult is a page of the library.
type BrowseResult struct {
	Hits   []index.Hit
	Total  int
	Query  index.Query
	Limit  int
	Offset int
}

// HasMore reports whether another page follows.
func (b BrowseResult) HasMore() bool { return b.Offset+len(b.Hits) < b.Total }

// Browse lists and searches the collection.
func (l *Library) Browse(ctx context.Context, q index.Query) (BrowseResult, error) {
	hits, total, err := l.idx.List(ctx, q)
	if err != nil {
		return BrowseResult{}, err
	}
	return BrowseResult{Hits: hits, Total: total, Query: q, Limit: q.Limit, Offset: q.Offset}, nil
}

// RecordView notes that a song was opened, which drives the recently-played
// list. A failure here is logged and swallowed: it must never stop a song from
// being read.
func (l *Library) RecordView(ctx context.Context, slug string) {
	if err := l.idx.RecordView(ctx, slug, l.now()); err != nil {
		l.log.Warn("could not record a view", "slug", slug, "error", err)
	}
}

// SaveState stores a reader's preferences for one song.
func (l *Library) SaveState(ctx context.Context, slug string, s index.SongState) error {
	return l.idx.SetState(ctx, slug, s)
}

// Tags lists the tags in use.
func (l *Library) Tags(ctx context.Context) ([]index.TagCount, error) { return l.idx.Tags(ctx) }

// Artists lists the artists in the collection.
func (l *Library) Artists(ctx context.Context) ([]index.ArtistCount, error) {
	return l.idx.Artists(ctx)
}

// Broken lists the files that will not parse.
func (l *Library) Broken(ctx context.Context) ([]index.Record, error) { return l.idx.Broken(ctx) }

// Stats summarises the collection.
func (l *Library) Stats(ctx context.Context) (index.Stats, error) { return l.idx.Stats(ctx) }

// Notation returns the stored default note names, or "" when none is set and
// the configured default should apply.
func (l *Library) Notation(ctx context.Context) (string, error) {
	return l.idx.Setting(ctx, SettingNotation)
}

// SetNotation stores the default note names.
func (l *Library) SetNotation(ctx context.Context, value string) error {
	return l.idx.SetSetting(ctx, SettingNotation, value)
}

// SyncIfNeeded runs a scan when the index has just been rebuilt and holds
// nothing, which is what happens on a first run or after the derived schema
// changed.
func (l *Library) SyncIfNeeded(ctx context.Context) (SyncReport, error) {
	if !l.idx.NeedsFullScan() {
		if s, err := l.idx.Stats(ctx); err == nil && s.Songs > 0 {
			return SyncReport{}, nil
		}
	}
	return l.Sync(ctx)
}

// StartPeriodicSync rescans on an interval until the context is cancelled.
//
// A ticker rather than filesystem notifications, for reasons beyond avoiding a
// dependency: watches are unreliable across bind mounts and network shares,
// editors that write-then-rename produce confusing event pairs, and the startup
// scan and the manual button are needed regardless — so a watcher would be a
// third code path that can disagree with the other two. Rescanning reuses
// exactly the same code, and Sync is cheap when nothing changed.
func (l *Library) StartPeriodicSync(ctx context.Context, every time.Duration) {
	if every <= 0 {
		l.log.Info("periodic rescanning is disabled")
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := l.Sync(ctx); err != nil && !errors.Is(err, ErrSyncBusy) &&
					!errors.Is(err, context.Canceled) {
					l.log.Error("periodic rescan failed", "error", err)
				}
			}
		}
	}()
}

// State returns a song's saved reading preferences.
func (l *Library) State(ctx context.Context, slug string) (index.SongState, error) {
	return l.idx.State(ctx, slug)
}
