package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/songfs"
)

type harness struct {
	lib   *Library
	idx   *index.Index
	store *songfs.Store
	songs string
	dbs   string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	base := t.TempDir()
	songs := filepath.Join(base, "songs")
	trash := filepath.Join(base, "trash")
	dbPath := filepath.Join(base, "index.db")

	store, err := songfs.Open(songs, trash, 1<<20)
	if err != nil {
		t.Fatalf("songfs.Open: %v", err)
	}
	idx, err := index.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { store.Close(); idx.Close() })

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &harness{lib: New(store, idx, quiet), idx: idx, store: store, songs: songs, dbs: dbPath}
}

// reopenIndex closes the index and opens it again at the same path, optionally
// deleting the file first to simulate a user throwing the database away.
func (h *harness) reopenIndex(t *testing.T, deleteFirst bool) {
	t.Helper()
	if err := h.idx.Close(); err != nil {
		t.Fatal(err)
	}
	if deleteFirst {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(h.dbs + suffix)
		}
	}
	idx, err := index.Open(context.Background(), h.dbs)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	h.idx = idx
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h.lib = New(h.store, idx, quiet)
}

func (h *harness) writeSong(t *testing.T, rel, title, artist, body string) {
	t.Helper()
	if body == "" {
		body = "## Verse 1\n[G]Hello [C]world\n"
	}
	content := fmt.Sprintf("---\ntitle: %s\n", title)
	if artist != "" {
		content += fmt.Sprintf("artist: %s\n", artist)
	}
	content += "---\n\n" + body
	full := filepath.Join(h.songs, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncAddsUpdatesAndRemoves(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	h.writeSong(t, "Traditional/Amazing Grace.song", "Amazing Grace", "Traditional", "")
	h.writeSong(t, "Bill Withers/Ain't No Sunshine.song", "Ain't No Sunshine", "Bill Withers", "")

	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if rep.Added != 2 || rep.Scanned != 2 {
		t.Errorf("first sync = %+v, want 2 added", rep)
	}

	// A second scan must find nothing to do.
	rep, err = h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 || rep.Updated != 0 || rep.Removed != 0 || rep.Unchanged != 2 {
		t.Errorf("second sync = %+v, want everything unchanged", rep)
	}

	// An edit that leaves the size identical must still be noticed, which is why
	// changes are detected by hash rather than by size and modification time.
	h.writeSong(t, "Traditional/Amazing Grace.song", "Amazing Grace", "Traditional",
		"## Verse 1\n[C]Hello [G]world\n")
	rep, err = h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 {
		t.Errorf("sync after a same-size edit = %+v, want 1 updated", rep)
	}
	view, err := h.lib.Get(ctx, index.SlugForPath("Traditional/Amazing Grace.song"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(chordSymbols(view.Record.Chords), " ") != "C G" {
		t.Errorf("chords = %v, want the edit to be reflected", view.Record.Chords)
	}

	// Removing the file removes the row.
	if err := os.Remove(filepath.Join(h.songs, "Bill Withers", "Ain't No Sunshine.song")); err != nil {
		t.Fatal(err)
	}
	rep, err = h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Removed != 1 {
		t.Errorf("sync after a delete = %+v, want 1 removed", rep)
	}
	if s, _ := h.lib.Stats(ctx); s.Songs != 1 {
		t.Errorf("collection holds %d songs, want 1", s.Songs)
	}
}

func chordSymbols(c []string) []string { return c }

// The architectural promise, end to end: throw the database away and everything
// that matters comes back, including the URLs.
func TestDeletingTheDatabaseLosesNothingButState(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	h.writeSong(t, "Traditional/Amazing Grace.song", "Amazing Grace", "Traditional", "")
	h.writeSong(t, "Joaquín Sabina/Y nos dieron las diez.song", "Y nos dieron las diez", "Joaquín Sabina", "")
	h.writeSong(t, "Loose.song", "Loose", "", "")
	// Two paths that slugify identically, to prove collisions resolve the same
	// way after a rebuild.
	h.writeSong(t, "A-B/C.song", "C", "A-B", "")
	h.writeSong(t, "A/B-C.song", "C", "A", "")

	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	before, _, err := h.idx.List(ctx, index.Query{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	beforeSlugs := make([]string, 0, len(before))
	for _, hit := range before {
		beforeSlugs = append(beforeSlugs, hit.RelPath+" => "+hit.Slug)
	}
	sort.Strings(beforeSlugs)
	if len(beforeSlugs) != 5 {
		t.Fatalf("indexed %d songs, want 5", len(beforeSlugs))
	}

	h.reopenIndex(t, true)
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatalf("sync after deleting the database: %v", err)
	}

	after, _, err := h.idx.List(ctx, index.Query{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	afterSlugs := make([]string, 0, len(after))
	for _, hit := range after {
		afterSlugs = append(afterSlugs, hit.RelPath+" => "+hit.Slug)
	}
	sort.Strings(afterSlugs)

	if strings.Join(beforeSlugs, "\n") != strings.Join(afterSlugs, "\n") {
		t.Errorf("slugs changed across a rebuild:\nbefore:\n%s\nafter:\n%s",
			strings.Join(beforeSlugs, "\n"), strings.Join(afterSlugs, "\n"))
	}
}

// Moving a song into a different folder must keep its saved transposition and
// its play count, and leave a redirect behind.
func TestSyncDetectsARenameAndKeepsState(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	h.writeSong(t, "Old Artist/Song.song", "Song", "Old Artist", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	oldSlug := index.SlugForPath("Old Artist/Song.song")
	if err := h.lib.SaveState(ctx, oldSlug, index.SongState{Transpose: 3}); err != nil {
		t.Fatal(err)
	}
	h.lib.RecordView(ctx, oldSlug)

	// Move it on disk, as a file manager would.
	if err := os.MkdirAll(filepath.Join(h.songs, "New Artist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(h.songs, "Old Artist", "Song.song"),
		filepath.Join(h.songs, "New Artist", "Song.song")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(h.songs, "Old Artist"))

	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Renamed != 1 {
		t.Errorf("sync = %+v, want 1 renamed", rep)
	}
	if rep.Removed != 0 || rep.Added != 0 || rep.Updated != 0 {
		t.Errorf("sync = %+v; a rename is one event, not a delete plus an add", rep)
	}

	newSlug := index.SlugForPath("New Artist/Song.song")
	st, err := h.idx.State(ctx, newSlug)
	if err != nil {
		t.Fatal(err)
	}
	if st.Transpose != 3 {
		t.Errorf("transposition did not follow the rename: %+v", st)
	}
	if st.ViewCount != 1 {
		t.Errorf("play count did not follow the rename: %+v", st)
	}

	// The old URL must redirect rather than 404.
	_, err = h.lib.Get(ctx, oldSlug)
	var moved ErrMoved
	if !errors.As(err, &moved) {
		t.Fatalf("Get(old slug) = %v, want ErrMoved", err)
	}
	if moved.NewSlug != newSlug {
		t.Errorf("redirect points at %q, want %q", moved.NewSlug, newSlug)
	}
}

// A duplicated file is not a rename, even though its content matches.
func TestSyncDoesNotMistakeACopyForARename(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/Song.song", "Song", "A", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	// Copy it, leaving the original in place.
	data, err := os.ReadFile(filepath.Join(h.songs, "A", "Song.song"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.songs, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.songs, "B", "Song.song"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Renamed != 0 {
		t.Errorf("sync = %+v, want the copy treated as new rather than a rename", rep)
	}
	if rep.Added != 1 {
		t.Errorf("sync = %+v, want 1 added", rep)
	}
	if s, _ := h.lib.Stats(ctx); s.Songs != 2 {
		t.Errorf("collection holds %d songs, want both the original and the copy", s.Songs)
	}
}

func TestBrokenFileStaysFindableAndKeepsLastGoodContent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	h.writeSong(t, "A/Song.song", "A Fine Title", "A", "## Verse 1\n[G]searchable words here\n")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	slug := index.SlugForPath("A/Song.song")

	// Break it, as a mistyped edit would.
	if err := os.WriteFile(filepath.Join(h.songs, "A", "Song.song"),
		[]byte("---\ntitel: oops\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 1 {
		t.Errorf("sync = %+v, want 1 failure reported", rep)
	}

	// The last good metadata is still there, so the song has not vanished.
	rec, found, err := h.idx.BySlug(ctx, slug)
	if err != nil || !found {
		t.Fatalf("the broken song is no longer indexed: found=%v err=%v", found, err)
	}
	if rec.Title != "A Fine Title" {
		t.Errorf("title = %q, want the last version that parsed", rec.Title)
	}
	if !rec.Broken() {
		t.Error("the record does not report the parse error")
	}
	if rec.ParseErrorLine == 0 {
		t.Error("no line number was recorded, so the editor cannot point at the mistake")
	}
	// And it is still searchable by its old content.
	if _, total, _ := h.idx.List(ctx, index.Query{Text: "searchable"}); total != 1 {
		t.Error("the broken song dropped out of search")
	}
	// Get surfaces the error while still returning the record.
	view, err := h.lib.Get(ctx, slug)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.ParseError == nil {
		t.Error("Get did not report that the file on disk is broken")
	}

	// Fixing the file clears the error.
	h.writeSong(t, "A/Song.song", "A Fine Title", "A", "## Verse 1\n[G]searchable words here\n")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	rec, _, _ = h.idx.BySlug(ctx, slug)
	if rec.Broken() {
		t.Errorf("the parse error survived the repair: %q", rec.ParseError)
	}
}

// A file that has never parsed still has to appear, or it looks like a lost song.
func TestNeverParsedFileIsStillIndexed(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	full := filepath.Join(h.songs, "Some Artist", "Broken Song.song")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("this was never a song"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 1 || rep.Added != 1 {
		t.Errorf("sync = %+v, want the file added and reported as failed", rep)
	}
	res, err := h.lib.Browse(ctx, index.Query{Broken: true})
	if err != nil {
		t.Fatal(err)
	}
	broken := res.Hits
	if len(broken) != 1 {
		t.Fatalf("listing broken songs gave %v, want the one file", broken)
	}
	// Falling back to the filename and folder means it is recognisable.
	if broken[0].Title != "Broken Song" {
		t.Errorf("title = %q, want it guessed from the filename", broken[0].Title)
	}
	if broken[0].Artist != "Some Artist" {
		t.Errorf("artist = %q, want it guessed from the folder", broken[0].Artist)
	}
}

func TestGetReadsContentFromTheFileNotTheIndex(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/Song.song", "Song", "A", "## Verse 1\n[G]original\n")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	// Change the file without telling the index.
	h.writeSong(t, "A/Song.song", "Song", "A", "## Verse 1\n[D]changed on disk\n")

	view, err := h.lib.Get(ctx, index.SlugForPath("A/Song.song"))
	if err != nil {
		t.Fatal(err)
	}
	got := view.Song.PlainText()
	if !strings.Contains(got, "changed on disk") {
		t.Errorf("Get returned stale content %q; the file is the source of truth", got)
	}
}

func TestGetMissingAndMoved(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	if _, err := h.lib.Get(ctx, "no-such-song"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(unknown) = %v, want ErrNotFound", err)
	}
	// A row whose file has gone reads as missing rather than as an error.
	h.writeSong(t, "A/Song.song", "Song", "A", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h.songs, "A", "Song.song")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.lib.Get(ctx, index.SlugForPath("A/Song.song")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(row whose file is gone) = %v, want ErrNotFound", err)
	}
}

func TestRawFile(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/Song.song", "Song", "A", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	data, hash, name, err := h.lib.RawFile(ctx, index.SlugForPath("A/Song.song"))
	if err != nil {
		t.Fatalf("RawFile: %v", err)
	}
	if !strings.Contains(string(data), "title: Song") {
		t.Errorf("RawFile returned %q", data)
	}
	if len(hash) != 64 {
		t.Errorf("hash = %q", hash)
	}
	if name != "Song.song" {
		t.Errorf("filename = %q, want Song.song", name)
	}
}

// Two scans must not run at once: the startup scan, the ticker and the reindex
// button would otherwise fight over the same rows.
func TestSyncIsSingleFlight(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/Song.song", "Song", "A", "")

	h.lib.syncing.Lock()
	_, err := h.lib.Sync(ctx)
	h.lib.syncing.Unlock()
	if !errors.Is(err, ErrSyncBusy) {
		t.Errorf("Sync while another is running = %v, want ErrSyncBusy", err)
	}
	// And it works again once the other finishes.
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Errorf("Sync after the lock was released: %v", err)
	}
}

func TestSyncHonoursCancellation(t *testing.T) {
	h := newHarness(t)
	for i := range 20 {
		h.writeSong(t, fmt.Sprintf("A/Song%02d.song", i), fmt.Sprint("Song ", i), "A", "")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.lib.Sync(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Sync with a cancelled context = %v, want context.Canceled", err)
	}
}

// Starting up rescans even when the index already holds songs, because that is
// exactly when the collection may have changed unobserved.
func TestSyncPicksUpAFileAddedWhileStopped(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/First.song", "First", "A", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// A file appears with nobody watching.
	h.writeSong(t, "B/Second.song", "Second", "B", "")

	rep, err := h.lib.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 1 {
		t.Errorf("sync = %+v, want the new file found", rep)
	}
	if s, _ := h.lib.Stats(ctx); s.Songs != 2 {
		t.Errorf("collection holds %d songs, want 2", s.Songs)
	}
}

func TestNotationSetting(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	if v, err := h.lib.Notation(ctx); err != nil || v != "" {
		t.Errorf("Notation = %q %v, want empty so the configured default applies", v, err)
	}
	if err := h.lib.SetNotation(ctx, "solfege"); err != nil {
		t.Fatal(err)
	}
	if v, _ := h.lib.Notation(ctx); v != "solfege" {
		t.Errorf("Notation = %q", v)
	}
}

func TestPeriodicSyncStopsWithTheContext(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.lib.StartPeriodicSync(ctx, 10*time.Millisecond)
	h.writeSong(t, "A/Song.song", "Song", "A", "")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := h.lib.Stats(context.Background()); s.Songs == 1 {
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Error("the periodic scan never indexed the new song")
}

// A disabled interval must not start a goroutine that scans anyway.
func TestPeriodicSyncCanBeDisabled(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.lib.StartPeriodicSync(ctx, 0)
	h.writeSong(t, "A/Song.song", "Song", "A", "")
	time.Sleep(50 * time.Millisecond)
	if s, _ := h.lib.Stats(context.Background()); s.Songs != 0 {
		t.Error("a disabled ticker scanned anyway")
	}
}
