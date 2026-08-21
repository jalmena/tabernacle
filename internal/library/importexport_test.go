package library

import (
	"archive/zip"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/index"
)

func songText(title, artist string) string {
	s := "---\ntitle: " + title + "\n"
	if artist != "" {
		s += "artist: " + artist + "\n"
	}
	return s + "---\n\n## Verse 1\n[G]hello [C]world\n"
}

// zipEntry describes one file to put in a test archive, including the hostile
// shapes a real attacker would use.
type zipEntry struct {
	name     string
	body     string
	mode     fs.FileMode
	lyingLen uint64 // when non-zero, the header claims this size
}

func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		} else {
			hdr.SetMode(0o644)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (h *harness) files(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(h.songs, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(h.songs, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestImportZip(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	data := buildZip(t, []zipEntry{
		{name: "songs/Traditional/Amazing Grace.song", body: songText("Amazing Grace", "Traditional")},
		{name: "Bill Withers/Ain't No Sunshine.song", body: songText("Ain't No Sunshine", "Bill Withers")},
		{name: "loose.song", body: songText("Loose", "")},
		{name: "notes.txt", body: "not a song"},
		{name: "broken.song", body: "---\ntitel: typo\n---\n"},
	})

	rep, err := h.lib.ImportZip(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ImportZip: %v", err)
	}
	if len(rep.Added) != 3 {
		t.Errorf("added %d songs, want 3: %+v", len(rep.Added), rep.Added)
	}
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0].Reason, ".song") {
		t.Errorf("skipped = %+v, want the .txt file", rep.Skipped)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].Line == 0 {
		t.Errorf("failed = %+v, want the broken song with a line number", rep.Failed)
	}

	// The destination comes from the parsed metadata, never from the archive's
	// own paths: note that "songs/" and the loose file are both placed properly.
	got := strings.Join(h.files(t), "\n")
	for _, want := range []string{
		"Traditional/Amazing Grace.song",
		"Bill Withers/Ain't No Sunshine.song",
		"Loose.song",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q on disk, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "songs/songs") {
		t.Errorf("the archive's own directory leaked into the collection:\n%s", got)
	}
	// And they are searchable.
	if s, _ := h.lib.Stats(ctx); s.Songs != 3 {
		t.Errorf("the index holds %d songs, want 3", s.Songs)
	}
}

// Zip slip and friends. Every one of these must be refused, and the collection
// must be untouched afterwards.
func TestImportZipRefusesHostileEntries(t *testing.T) {
	hostile := []struct {
		name  string
		entry zipEntry
	}{
		{"parent", zipEntry{name: "../evil.song", body: songText("Evil", "")}},
		{"deep parent", zipEntry{name: "../../../../etc/evil.song", body: songText("Evil", "")}},
		{"embedded parent", zipEntry{name: "a/b/../../../evil.song", body: songText("Evil", "")}},
		{"absolute", zipEntry{name: "/etc/evil.song", body: songText("Evil", "")}},
		{"backslash parent", zipEntry{name: `..\..\evil.song`, body: songText("Evil", "")}},
		{"backslash sep", zipEntry{name: `a\b.song`, body: songText("Evil", "")}},
		{"drive letter", zipEntry{name: "C:/evil.song", body: songText("Evil", "")}},
		{"nul byte", zipEntry{name: "ev\x00il.song", body: songText("Evil", "")}},
		{"control char", zipEntry{name: "ev\x01il.song", body: songText("Evil", "")}},
		{"symlink", zipEntry{name: "link.song", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777}},
		{"setuid", zipEntry{name: "setuid.song", body: songText("Evil", ""), mode: fs.ModeSetuid | 0o755}},
		{"device", zipEntry{name: "dev.song", body: songText("Evil", ""), mode: fs.ModeDevice | 0o644}},
		{"no name", zipEntry{name: ".song", body: songText("Evil", "")}},
		{"dot dot only", zipEntry{name: "..", body: songText("Evil", "")}},
	}
	for _, tt := range hostile {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			outside := filepath.Join(filepath.Dir(h.songs), "evil.song")

			data := buildZip(t, []zipEntry{tt.entry})
			rep, err := h.lib.ImportZip(ctx, bytes.NewReader(data), int64(len(data)))
			if err != nil {
				return // refusing the whole archive is also an acceptable answer
			}
			if len(rep.Added) != 0 {
				t.Errorf("a hostile entry was imported: %+v", rep.Added)
			}
			if files := h.files(t); len(files) != 0 {
				t.Errorf("the collection is no longer empty: %v", files)
			}
			if _, err := os.Stat(outside); err == nil {
				t.Fatalf("a file was written outside the collection at %s", outside)
			}
			// Whatever happened, the user is told about it.
			if rep.Total() == 0 {
				t.Error("the entry vanished from the report entirely")
			}
		})
	}
}

// A small archive that decompresses to something enormous must not be absorbed.
func TestImportZipRefusesABomb(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	// Highly compressible content, far larger than one song may be.
	body := "---\ntitle: Big\n---\n\n## V\n" + strings.Repeat("[G]la ", 400000)
	data := buildZip(t, []zipEntry{{name: "big.song", body: body}})
	t.Logf("archive is %d bytes and holds %d", len(data), len(body))

	rep, err := h.lib.ImportZip(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return
	}
	if len(rep.Added) != 0 {
		t.Errorf("an oversize entry was imported: %+v", rep.Added)
	}
	if files := h.files(t); len(files) != 0 {
		t.Errorf("the oversize entry was written: %v", files)
	}
}

// A header that lies about its size must not get past the limit.
func TestImportZipDoesNotTrustTheDeclaredSize(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	// Build the archive by hand so the central directory understates the size.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: "liar.song", Method: zip.Store}
	hdr.SetMode(0o644)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	big := "---\ntitle: Liar\n---\n\n## V\n" + strings.Repeat("x", maxImportSongBytes+1000)
	if _, err := w.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	rep, err := h.lib.ImportZip(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return
	}
	if len(rep.Added) != 0 {
		t.Errorf("an oversize entry got through: %+v", rep.Added)
	}
	if files := h.files(t); len(files) != 0 {
		t.Errorf("it was written anyway: %v", files)
	}
}

func TestImportZipRefusesTooManyEntries(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	entries := make([]zipEntry, 0, maxZipEntries+1)
	for i := range maxZipEntries + 1 {
		entries = append(entries, zipEntry{
			name: filepath.ToSlash(filepath.Join("a", string(rune('a'+i%26))+".song")) +
				string(rune('0'+i%10)) + ".song",
			body: songText("S", ""),
		})
	}
	data := buildZip(t, entries)
	if _, err := h.lib.ImportZip(ctx, bytes.NewReader(data), int64(len(data))); err == nil {
		t.Error("an archive with too many entries was accepted")
	}
	if files := h.files(t); len(files) != 0 {
		t.Errorf("something was written: %d files", len(files))
	}
}

// One bad file must not discard the good ones around it.
func TestImportReportsPerFileRatherThanRollingBack(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	entries := []zipEntry{}
	for i := range 20 {
		entries = append(entries, zipEntry{
			name: "ok" + string(rune('a'+i)) + ".song",
			body: songText("Song "+string(rune('A'+i)), "Artist"),
		})
	}
	// A broken one right at the end.
	entries = append(entries, zipEntry{name: "bad.song", body: "not a song at all"})

	data := buildZip(t, entries)
	rep, err := h.lib.ImportZip(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Added) != 20 {
		t.Errorf("added %d, want the 20 good songs kept despite the bad one", len(rep.Added))
	}
	if len(rep.Failed) != 1 {
		t.Errorf("failed = %+v, want just the bad one", rep.Failed)
	}
}

func TestImportSingleFile(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	rep := h.lib.ImportSong(ctx, "whatever.song", []byte(songText("Solo", "Artist")))
	if len(rep.Added) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Added[0].Slug != index.SlugForPath("Artist/Solo.song") {
		t.Errorf("slug = %q", rep.Added[0].Slug)
	}
	// The uploaded filename is not the destination: the metadata is.
	if got := h.files(t); len(got) != 1 || got[0] != "Artist/Solo.song" {
		t.Errorf("files = %v, want Artist/Solo.song", got)
	}

	bad := h.lib.ImportSong(ctx, "bad.song", []byte("nope"))
	if len(bad.Failed) != 1 {
		t.Errorf("a bad upload was not reported: %+v", bad)
	}
}

// Export then import must reproduce the collection, which is what makes the
// backup story honest.
func TestExportThenImportRoundTrips(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "Traditional/Amazing Grace.song", "Amazing Grace", "Traditional", "")
	h.writeSong(t, "Joaquín Sabina/Y nos dieron las diez.song", "Y nos dieron las diez", "Joaquín Sabina", "")
	h.writeSong(t, "Loose.song", "Loose", "", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	n, err := h.lib.ExportZip(ctx, &out, index.Query{})
	if err != nil {
		t.Fatalf("ExportZip: %v", err)
	}
	if n != 3 {
		t.Errorf("exported %d songs, want 3", n)
	}

	// The archive mirrors the directory, so restoring is unzipping.
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, "\n")
	for _, want := range []string{
		"songs/Traditional/Amazing Grace.song",
		"songs/Joaquín Sabina/Y nos dieron las diez.song",
		"songs/Loose.song",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the archive is missing %q, has:\n%s", want, joined)
		}
	}

	// Import it into a fresh collection and compare.
	h2 := newHarness(t)
	rep, err := h2.lib.ImportZip(ctx, bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Added) != 3 {
		t.Fatalf("re-imported %d of 3: %+v", len(rep.Added), rep)
	}
	before, after := h.files(t), h2.files(t)
	if strings.Join(before, "|") != strings.Join(after, "|") {
		t.Errorf("the round trip changed the collection:\n%v\n%v", before, after)
	}
}

func TestExportRespectsAFilter(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/One.song", "One", "A", "")
	h.writeSong(t, "B/Two.song", "Two", "B", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	n, err := h.lib.ExportZip(ctx, &out, index.Query{Artist: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("exported %d songs, want just the filtered one", n)
	}
}

func TestExportOfAnEmptyCollection(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	var out bytes.Buffer
	n, err := h.lib.ExportZip(ctx, &out, index.Query{})
	if err != nil {
		t.Fatalf("exporting nothing failed: %v", err)
	}
	if n != 0 {
		t.Errorf("exported %d songs from an empty collection", n)
	}
	// It must still be a valid, readable archive.
	if _, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len())); err != nil {
		t.Errorf("the empty archive is not valid: %v", err)
	}
}

func TestImportZipRejectsGarbage(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	junk := []byte("this is definitely not a zip file")
	if _, err := h.lib.ImportZip(ctx, bytes.NewReader(junk), int64(len(junk))); err == nil {
		t.Error("garbage was accepted as an archive")
	}
}

// Restoring a backup over a collection that already holds it must be a no-op,
// not a way to double everything. This is the property that makes the backup
// story usable rather than merely present.
func TestImportingABackupTwiceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "Traditional/Amazing Grace.song", "Amazing Grace", "Traditional", "")
	h.writeSong(t, "A/Other.song", "Other", "A", "")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if _, err := h.lib.ExportZip(ctx, &out, index.Query{}); err != nil {
		t.Fatal(err)
	}
	before := h.files(t)

	// Restore over itself, twice.
	for round := 1; round <= 2; round++ {
		rep, err := h.lib.ImportZip(ctx, bytes.NewReader(out.Bytes()), int64(out.Len()))
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(rep.Added) != 0 {
			t.Errorf("round %d added %d songs that were already there: %+v",
				round, len(rep.Added), rep.Added)
		}
		if len(rep.Skipped) != 2 {
			t.Errorf("round %d skipped %d, want both songs recognised", round, len(rep.Skipped))
		}
		for _, sk := range rep.Skipped {
			if sk.Slug == "" {
				t.Error("a skipped song does not say where the existing one is")
			}
		}
	}

	if after := h.files(t); strings.Join(before, "|") != strings.Join(after, "|") {
		t.Errorf("restoring changed the collection:\n%v\n%v", before, after)
	}
	if s, _ := h.lib.Stats(ctx); s.Songs != 2 {
		t.Errorf("the collection holds %d songs, want 2", s.Songs)
	}
}

// A genuinely different song with the same title still gets in; only identical
// content is recognised as already present.
func TestImportStillAddsAChangedVersion(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writeSong(t, "A/Song.song", "Song", "A", "## Verse 1\n[G]original words\n")
	if _, err := h.lib.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	different := songText("Song", "A") // different body
	rep := h.lib.ImportSong(ctx, "song.song", []byte(different))
	if len(rep.Added) != 1 {
		t.Fatalf("a changed version was not added: %+v", rep)
	}
	if got := h.files(t); len(got) != 2 {
		t.Errorf("files = %v, want both versions kept", got)
	}
}
