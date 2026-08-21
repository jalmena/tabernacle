package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The promise the whole design rests on: the .song files are the collection and
// SQLite is a cache. Delete the database, start again, and nothing is lost —
// not the songs, and not the addresses they live at.
//
// If this test ever fails, the project has stopped being what it claims to be.
func TestTheIndexIsDisposable(t *testing.T) {
	in := start(t, nil)
	in.seed()

	// Take the collection as the reader sees it, addresses included.
	before := in.page("/")
	urls := []string{
		"/songs/traditional-amazing-grace",
		"/songs/prueba-jazz",
		"/songs/otro-con-cejilla",
	}
	sheets := map[string]string{}
	for _, u := range urls {
		sheets[u] = in.page(u)
	}

	// Now throw the index away, as a corrupted volume or a curious user would.
	in.stop()
	matches, err := filepath.Glob(filepath.Join(in.dir, "*.db*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no database was found to delete, so this test proves nothing")
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}
	in.boot()

	// Every address still answers, with the same sheet.
	for _, u := range urls {
		res := in.get(u)
		if res.Status != http.StatusOK {
			t.Errorf("after rebuilding the index, %s answers %d", u, res.Status)
			continue
		}
		if res.Body != sheets[u] {
			t.Errorf("%s renders differently after the index was rebuilt", u)
		}
	}
	if got := in.page("/"); got != before {
		t.Error("the library page differs after the index was rebuilt")
	}
}

// A restart alone must change nothing. This is separate from the test above
// because "survives a restart" and "survives losing the index" fail for
// different reasons.
func TestARestartChangesNothing(t *testing.T) {
	in := start(t, nil)
	in.seed()

	before := in.page("/songs/traditional-amazing-grace")
	in.restart()
	if after := in.page("/songs/traditional-amazing-grace"); after != before {
		t.Error("the sheet changed across a restart")
	}
}

// A song copied into the volume while the application was down must be there
// when it comes back, without anybody asking for a scan.
//
// It was not, once: startup skipped the scan when the index already had rows, so
// a file copied in during downtime waited for the next timer tick.
func TestASongAddedWhileStoppedIsFoundAtStartup(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.stop()

	path := filepath.Join(in.songs, "Nadie", "Mientras dormía.song")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Mientras dormía\nartist: Nadie\nkey: A\n---\n\n## Estrofa 1\n[A]uno\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	in.boot()
	in.get("/").mustContain("Mientras dormía")
	if in.get("/songs/nadie-mientras-dormia").Status != http.StatusOK {
		t.Error("the song has no page")
	}
}

// The file wins. Editing a .song in a text editor is a supported way to use
// this, so the change has to show up.
func TestTheFileOnDiskIsTheTruth(t *testing.T) {
	in := start(t, nil)
	in.seed()

	in.get("/songs/traditional-amazing-grace").mustContain("sweet")

	path := filepath.Join(in.songs, "Traditional", "Amazing Grace.song")
	edited := strings.Replace(amazingGrace, "sweet", "editado a mano", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	in.rescan()

	res := in.get("/songs/traditional-amazing-grace")
	res.mustContain("editado")
	res.mustNotContain("sweet")
}

// And a file taken away goes away, rather than lingering as a page that leads to
// nothing.
func TestAFileRemovedOnDiskLeavesTheLibrary(t *testing.T) {
	in := start(t, nil)
	in.seed()

	if err := os.Remove(filepath.Join(in.songs, "Traditional", "Amazing Grace.song")); err != nil {
		t.Fatal(err)
	}
	in.rescan()

	in.get("/").mustNotContain("Amazing Grace")
	if got := in.get("/songs/traditional-amazing-grace").Status; got != http.StatusNotFound {
		t.Errorf("the removed song's page answers %d, want 404", got)
	}
}

// A change to a file that keeps its size must still be noticed, which is why
// the index compares content and not the cheap metadata.
func TestAChangeOfTheSameLengthIsNoticed(t *testing.T) {
	in := start(t, nil)
	in.seed()

	path := filepath.Join(in.songs, "Traditional", "Amazing Grace.song")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// "sweet" and "SWEET" are the same length, and on a coarse filesystem the
	// modification time can be the same too.
	swapped := strings.Replace(string(original), "sweet", "aaaaa", 1)
	if len(swapped) != len(original) {
		t.Fatalf("the test's own substitution changed the length")
	}
	if err := os.WriteFile(path, []byte(swapped), 0o644); err != nil {
		t.Fatal(err)
	}
	in.rescan()

	in.get("/songs/traditional-amazing-grace").mustContain("aaaaa")
}

// Reindexing by hand is offered, and it must be a no-op on an already-correct
// index rather than a way to lose things.
func TestReindexingIsHarmless(t *testing.T) {
	in := start(t, nil)
	in.seed()

	before := in.page("/")
	res := in.post("/reindex", nil)
	if res.Status != http.StatusSeeOther && res.Status != http.StatusOK {
		t.Fatalf("reindex answered %d", res.Status)
	}
	if after := in.page("/"); after != before {
		t.Error("reindexing changed the library page")
	}
}

// A file that does not parse must not stop the rest of the collection being
// served. One bad file in a directory of two hundred cannot take the songbook
// down.
func TestABrokenFileDoesNotBreakTheCollection(t *testing.T) {
	in := start(t, nil)
	in.seed()

	path := filepath.Join(in.songs, "Roto", "roto.song")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\ntitel: no\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.rescan()

	in.get("/").mustContain("Amazing Grace", "Jazz", "Con cejilla")
	in.restart()
	in.get("/").mustContain("Amazing Grace")
}
