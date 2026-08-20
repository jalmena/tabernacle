package index

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newIndex opens an index in a real temporary file rather than in memory, so
// that WAL, busy_timeout and the foreign keys are actually exercised.
func newIndex(t *testing.T) (*Index, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	x, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { x.Close() })
	return x, path
}

func rec(rel, title, artist string) Record {
	return Record{
		RelPath: rel,
		Slug:    SlugForPath(rel),
		Title:   title,
		Artist:  artist,
		Hash:    fmt.Sprintf("%064x", len(rel)),
		Size:    int64(len(rel)),
		ModTime: time.Unix(1700000000, 0),
	}
}

func TestOpenAppliesSchemaAndReportsFirstRun(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.db")

	x, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !x.NeedsFullScan() {
		t.Error("a brand new index should ask for a full scan")
	}
	if err := x.SetSetting(ctx, "notation", "solfege"); err != nil {
		t.Fatal(err)
	}
	x.Close()

	// Reopening must be a no-op: no migration reruns, no rebuild.
	x2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer x2.Close()
	if x2.NeedsFullScan() {
		t.Error("reopening an up-to-date index asked for a full scan")
	}
	if v, _ := x2.Setting(ctx, "notation"); v != "solfege" {
		t.Errorf("setting did not survive a reopen: %q", v)
	}
}

func TestUpsertReadAndUpdate(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)

	r := rec("Traditional/Amazing Grace.song", "Amazing Grace", "Traditional")
	r.Key = "G"
	r.Capo = 2
	r.Tuning = "EADGBE"
	r.Tempo = 90
	r.Meter = "3/4"
	r.Notes = "Public domain"
	r.Tags = []string{"hymn", "public-domain"}
	r.Chords = []string{"G", "C", "Em", "D"}
	r.Lyrics = "Amazing grace how sweet the sound"
	r.Sections = 3
	r.Lines = 12

	id, err := x.Upsert(ctx, r)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, ok, err := x.BySlug(ctx, r.Slug)
	if err != nil || !ok {
		t.Fatalf("BySlug: ok=%v err=%v", ok, err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	for _, c := range []struct {
		name      string
		got, want any
	}{
		{"title", got.Title, "Amazing Grace"},
		{"artist", got.Artist, "Traditional"},
		{"key", got.Key, "G"},
		{"capo", got.Capo, 2},
		{"tempo", got.Tempo, 90},
		{"meter", got.Meter, "3/4"},
		{"sections", got.Sections, 3},
		{"lines", got.Lines, 12},
	} {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	// Chords keep the order they first appeared in; tags are sorted for display.
	if strings.Join(got.Chords, " ") != "G C Em D" {
		t.Errorf("chords = %v, want first-appearance order", got.Chords)
	}
	if strings.Join(got.Tags, " ") != "hymn public-domain" {
		t.Errorf("tags = %v", got.Tags)
	}

	// Updating the same path must reuse the row rather than making a second one.
	r.Title = "Amazing Grace (revised)"
	r.Tags = []string{"hymn"}
	r.Chords = []string{"G", "D"}
	id2, err := x.Upsert(ctx, r)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if id2 != id {
		t.Errorf("upsert created a new row: %d then %d", id, id2)
	}
	got, _, _ = x.BySlug(ctx, r.Slug)
	if got.Title != "Amazing Grace (revised)" {
		t.Errorf("title was not updated: %q", got.Title)
	}
	if strings.Join(got.Tags, " ") != "hymn" {
		t.Errorf("tags were not replaced: %v", got.Tags)
	}
	if strings.Join(got.Chords, " ") != "G D" {
		t.Errorf("chords were not replaced: %v", got.Chords)
	}
	if s, _ := x.Stats(ctx); s.Songs != 1 {
		t.Errorf("collection holds %d songs, want 1", s.Songs)
	}
}

func TestDeleteRemovesDerivedRows(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	r := rec("A/Song.song", "Song", "A")
	r.Tags = []string{"rock"}
	r.Chords = []string{"E"}
	r.Lyrics = "unmistakable lyric"
	if _, err := x.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := x.DeleteByRelPath(ctx, r.RelPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := x.BySlug(ctx, r.Slug); ok {
		t.Error("the song is still readable after deletion")
	}
	// The search rows must go too, or a deleted song keeps turning up.
	hits, total, err := x.List(ctx, Query{Text: "unmistakable"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(hits) != 0 {
		t.Errorf("search still finds the deleted song: %d hits", total)
	}
	if tags, _ := x.Tags(ctx); len(tags) != 0 {
		t.Errorf("orphan tags left behind: %v", tags)
	}
	// Deleting something absent is not an error.
	if err := x.DeleteByRelPath(ctx, "nope.song"); err != nil {
		t.Errorf("deleting a missing song: %v", err)
	}
}

func TestStamps(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	a := rec("A/One.song", "One", "A")
	b := rec("B/Two.song", "Two", "B")
	if _, err := x.Upsert(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Upsert(ctx, b); err != nil {
		t.Fatal(err)
	}
	stamps, err := x.Stamps(ctx)
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	if len(stamps) != 2 {
		t.Fatalf("got %d stamps, want 2", len(stamps))
	}
	s := stamps["A/One.song"]
	if s.Hash != a.Hash || s.Slug != a.Slug || s.Size != a.Size {
		t.Errorf("stamp = %+v, does not match the record", s)
	}
	if !s.ModTime.Equal(a.ModTime) {
		t.Errorf("stamp mtime = %v, want %v", s.ModTime, a.ModTime)
	}
}

func TestByHashFindsARename(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	r := rec("Old/Song.song", "Song", "Old")
	if _, err := x.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := x.ByHash(ctx, r.Hash)
	if err != nil || !ok {
		t.Fatalf("ByHash: ok=%v err=%v", ok, err)
	}
	if got.RelPath != r.RelPath {
		t.Errorf("path = %q", got.RelPath)
	}
	if _, ok, _ := x.ByHash(ctx, strings.Repeat("f", 64)); ok {
		t.Error("ByHash matched an unknown hash")
	}
}

func TestUpdatePathKeepsIdentity(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	r := rec("Old/Song.song", "Song", "Old")
	id, err := x.Upsert(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	newRel := "New/Song.song"
	newSlug := SlugForPath(newRel)
	if err := x.UpdatePath(ctx, id, newRel, newSlug); err != nil {
		t.Fatalf("UpdatePath: %v", err)
	}
	got, ok, _ := x.BySlug(ctx, newSlug)
	if !ok || got.ID != id {
		t.Errorf("after a move: ok=%v id=%d want %d", ok, got.ID, id)
	}
	if _, ok, _ := x.BySlug(ctx, r.Slug); ok {
		t.Error("the song still answers on its old slug")
	}
}

func TestBrokenFilesStayVisible(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	good := rec("A/Good.song", "Good", "A")
	bad := rec("A/Bad.song", "Bad", "A")
	bad.ParseError = `unknown field "titel"`
	bad.ParseErrorLine = 3
	for _, r := range []Record{good, bad} {
		if _, err := x.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	broken, err := x.Broken(ctx)
	if err != nil {
		t.Fatalf("Broken: %v", err)
	}
	if len(broken) != 1 || broken[0].RelPath != "A/Bad.song" {
		t.Fatalf("Broken = %v, want just the bad file", broken)
	}
	if broken[0].ParseErrorLine != 3 {
		t.Errorf("line = %d, want 3", broken[0].ParseErrorLine)
	}
	if !broken[0].Broken() {
		t.Error("Broken() reports false for a file with a parse error")
	}
	// A broken song is still listable, so it can be found and fixed.
	if _, total, _ := x.List(ctx, Query{Broken: true}); total != 1 {
		t.Errorf("filtering for broken songs found %d", total)
	}
	if s, _ := x.Stats(ctx); s.Broken != 1 || s.Songs != 2 {
		t.Errorf("stats = %+v", s)
	}
}

func TestFavoriteComesFromTheTag(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	fav := rec("A/Loved.song", "Loved", "A")
	fav.Tags = []string{"rock", FavoriteTag}
	plain := rec("A/Plain.song", "Plain", "A")
	plain.Tags = []string{"rock"}
	for _, r := range []Record{fav, plain} {
		if _, err := x.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, _, _ := x.BySlug(ctx, fav.Slug)
	if !got.Favorite() {
		t.Error("the tagged song is not reported as a favourite")
	}
	got, _, _ = x.BySlug(ctx, plain.Slug)
	if got.Favorite() {
		t.Error("an untagged song is reported as a favourite")
	}
	hits, total, err := x.List(ctx, Query{Favorite: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || hits[0].Title != "Loved" {
		t.Errorf("favourites filter returned %d hits", total)
	}
}

// The architectural promise: the derived tables can be thrown away and rebuilt,
// and nothing that is not in the song files is lost.
func TestDerivedRebuildKeepsDurableState(t *testing.T) {
	ctx := context.Background()
	x, path := newIndex(t)

	r := rec("Traditional/Amazing Grace.song", "Amazing Grace", "Traditional")
	if _, err := x.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := x.SetState(ctx, r.Slug, SongState{Transpose: 2, Notation: "solfege"}); err != nil {
		t.Fatal(err)
	}
	if err := x.RecordView(ctx, r.Slug, time.Unix(1700000500, 0)); err != nil {
		t.Fatal(err)
	}
	if err := x.SetSetting(ctx, "notation", "solfege"); err != nil {
		t.Fatal(err)
	}
	if err := x.MoveState(ctx, "an-old-slug", r.Slug); err != nil {
		t.Fatal(err)
	}

	// Pretend the derived schema changed under us.
	if err := x.SetSetting(ctx, settingDerivedVersion, "0"); err != nil {
		t.Fatal(err)
	}
	x.Close()

	x2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer x2.Close()

	if !x2.NeedsFullScan() {
		t.Error("a changed derived schema should ask for a full scan")
	}
	// Derived data is gone, as intended: it comes back from the files.
	if s, _ := x2.Stats(ctx); s.Songs != 0 {
		t.Errorf("derived tables were not rebuilt: %d songs remain", s.Songs)
	}
	// Durable state survived.
	st, err := x2.State(ctx, r.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if st.Transpose != 2 || st.Notation != "solfege" {
		t.Errorf("reading state was lost: %+v", st)
	}
	if st.ViewCount != 1 {
		t.Errorf("view count = %d, want 1", st.ViewCount)
	}
	if v, _ := x2.Setting(ctx, "notation"); v != "solfege" {
		t.Errorf("global setting was lost: %q", v)
	}
	if to, ok, _ := x2.Redirect(ctx, "an-old-slug"); !ok || to != r.Slug {
		t.Errorf("redirect was lost: %q ok=%v", to, ok)
	}
}

func TestStateDefaultsAndUpdates(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)

	// A song never opened has a zero state, not an error.
	st, err := x.State(ctx, "never-seen")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st != (SongState{}) {
		t.Errorf("state = %+v, want the zero value", st)
	}

	if err := x.SetState(ctx, "s", SongState{Transpose: -3, CapoView: "played", Simplify: true}); err != nil {
		t.Fatal(err)
	}
	if err := x.RecordView(ctx, "s", time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := x.RecordView(ctx, "s", time.Unix(1700000060, 0)); err != nil {
		t.Fatal(err)
	}
	st, _ = x.State(ctx, "s")
	if st.Transpose != -3 || st.CapoView != "played" || !st.Simplify {
		t.Errorf("state = %+v", st)
	}
	if st.ViewCount != 2 {
		t.Errorf("view count = %d, want 2", st.ViewCount)
	}
	if !st.LastViewed.Equal(time.Unix(1700000060, 0)) {
		t.Errorf("last viewed = %v", st.LastViewed)
	}
	// Saving preferences must not reset the counters.
	if err := x.SetState(ctx, "s", SongState{Transpose: 1}); err != nil {
		t.Fatal(err)
	}
	st, _ = x.State(ctx, "s")
	if st.ViewCount != 2 {
		t.Errorf("view count = %d after saving preferences, want 2", st.ViewCount)
	}
}

func TestMoveStateAndRedirects(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	if err := x.SetState(ctx, "old", SongState{Transpose: 5}); err != nil {
		t.Fatal(err)
	}
	if err := x.MoveState(ctx, "old", "new"); err != nil {
		t.Fatalf("MoveState: %v", err)
	}
	st, _ := x.State(ctx, "new")
	if st.Transpose != 5 {
		t.Errorf("state did not follow the rename: %+v", st)
	}
	if st, _ := x.State(ctx, "old"); st.Transpose != 0 {
		t.Error("state was left behind at the old slug")
	}
	if to, ok, _ := x.Redirect(ctx, "old"); !ok || to != "new" {
		t.Errorf("Redirect(old) = %q ok=%v, want new", to, ok)
	}

	// Renamed twice: the first slug should point straight at the final one
	// rather than needing two hops.
	if err := x.MoveState(ctx, "new", "newest"); err != nil {
		t.Fatal(err)
	}
	if to, ok, _ := x.Redirect(ctx, "old"); !ok || to != "newest" {
		t.Errorf("Redirect(old) = %q, want newest after a second rename", to)
	}
	if _, ok, _ := x.Redirect(ctx, "unknown"); ok {
		t.Error("Redirect invented a destination for an unknown slug")
	}
}

// A song restored from the trash must answer on its own URL again, not redirect
// to whatever replaced it.
func TestUpsertClearsAStaleRedirect(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	r := rec("A/Song.song", "Song", "A")
	if err := x.MoveState(ctx, r.Slug, "somewhere-else"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := x.Redirect(ctx, r.Slug); !ok {
		t.Fatal("expected a redirect to exist")
	}
	if _, err := x.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	if to, ok, _ := x.Redirect(ctx, r.Slug); ok {
		t.Errorf("the redirect to %q survived the song coming back", to)
	}
}

func TestUpsertRejectsIncompleteRecords(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	if _, err := x.Upsert(ctx, Record{Slug: "s"}); err == nil {
		t.Error("Upsert accepted a record with no path")
	}
	if _, err := x.Upsert(ctx, Record{RelPath: "a.song"}); err == nil {
		t.Error("Upsert accepted a record with no slug")
	}
}

func TestOpenFailsOnAnUnusablePath(t *testing.T) {
	// A directory where the database file should be.
	dir := t.TempDir()
	if _, err := Open(context.Background(), dir); err == nil {
		t.Error("Open succeeded against a directory")
	} else if !strings.Contains(err.Error(), "index:") {
		t.Errorf("error %v should name the package", err)
	}
}

func TestArtists(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	for _, r := range []Record{
		rec("The Zombies/A.song", "A", "The Zombies"),
		rec("Adele/B.song", "B", "Adele"),
		rec("Adele/C.song", "C", "Adele"),
		rec("Loose.song", "D", ""),
	} {
		if _, err := x.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	artists, err := x.Artists(ctx)
	if err != nil {
		t.Fatalf("Artists: %v", err)
	}
	if len(artists) != 2 {
		t.Fatalf("got %v, want two named artists", artists)
	}
	// Sorted by the article-stripped key, so Adele precedes The Zombies.
	if artists[0].Name != "Adele" || artists[0].Count != 2 {
		t.Errorf("first artist = %+v", artists[0])
	}
	if artists[1].Name != "The Zombies" {
		t.Errorf("second artist = %+v", artists[1])
	}
}

func timeNow() time.Time { return time.Unix(1700001000, 0) }

// newIndexForFuzz opens an index for a fuzz target, where *testing.F is not a
// *testing.T but offers the same TempDir and Cleanup.
func newIndexForFuzz(f *testing.F) (*Index, string) {
	f.Helper()
	path := filepath.Join(f.TempDir(), "index.db")
	x, err := Open(context.Background(), path)
	if err != nil {
		f.Fatalf("Open: %v", err)
	}
	f.Cleanup(func() { x.Close() })
	return x, path
}
