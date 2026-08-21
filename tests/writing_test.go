package tests

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hashField = regexp.MustCompile(`name="hash"\s+value="([^"]*)"`)

// editHash reads the optimistic-concurrency token out of the edit form, the same
// way the browser posts it back.
func (in *instance) editHash(slug string) string {
	in.t.Helper()
	body := in.page("/songs/" + slug + "/edit")
	m := hashField.FindStringSubmatch(body)
	if m == nil {
		in.t.Fatalf("the edit form for %s carries no hash", slug)
	}
	return m[1]
}

const newSong = `---
title: Una nueva
artist: Alguien
key: D
genre: rock
---

## Estrofa 1
[D]uno [A]dos [Bm]tres [G]cuatro
`

// The whole journey, over HTTP, ending at the files on disk: a song is created,
// edited and deleted, and each step is visible both on the page and in the
// directory that is the source of truth.
func TestCreateEditDeleteJourney(t *testing.T) {
	in := start(t, nil)

	// --- create ---
	res := in.post("/songs", url.Values{"text": {newSong}})
	loc := res.location()
	if !strings.HasPrefix(loc, "/songs/") {
		t.Fatalf("created a song and went to %q", loc)
	}
	slug := strings.TrimPrefix(strings.SplitN(loc, "?", 2)[0], "/songs/")

	// It is on disk, in a directory named after the artist, and the file is the
	// canonical form rather than whatever was posted.
	files := in.songFiles()
	if len(files) != 1 || files[0] != "Alguien/Una nueva.song" {
		t.Fatalf("the collection on disk is %v, want [Alguien/Una nueva.song]", files)
	}
	onDisk, err := os.ReadFile(filepath.Join(in.songs, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), "title: Una nueva") {
		t.Errorf("the file does not hold the song:\n%s", onDisk)
	}

	// And the reader is told it worked, on the song's own page.
	page := in.get(loc)
	if page.Status != http.StatusOK {
		t.Fatalf("the new song's page answers %d", page.Status)
	}
	page.mustContain("Una nueva", "Bm", `role="status"`)

	// --- edit ---
	edited := strings.Replace(newSong, "[D]uno", "[D]uno cambiado", 1)
	res = in.post("/songs/"+slug, url.Values{
		"text": {edited},
		"hash": {in.editHash(slug)},
	})
	if loc := res.location(); !strings.HasPrefix(loc, "/songs/") {
		t.Fatalf("saved and went to %q", loc)
	}
	in.get("/songs/" + slug).mustContain("cambiado")
	if after, err := os.ReadFile(filepath.Join(in.songs, files[0])); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(after), "cambiado") {
		t.Error("the edit did not reach the file")
	}

	// --- delete ---
	res = in.post("/songs/"+slug+"/delete", nil)
	if loc := res.location(); !strings.HasPrefix(loc, "/") {
		t.Fatalf("deleted and went to %q", loc)
	}
	if got := in.songFiles(); len(got) != 0 {
		t.Errorf("the file is still in the collection: %v", got)
	}
	if in.get("/songs/"+slug).Status != http.StatusNotFound {
		t.Error("the deleted song's page still answers")
	}
	in.get("/").mustNotContain("Una nueva")

	// Deleting moves the file to the trash rather than destroying it. A songbook
	// that can lose work to a misclick is not one to trust.
	trash, err := filepath.Glob(filepath.Join(in.dir, "trash", "*.song"))
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) == 0 {
		t.Error("the deleted song is not in the trash")
	}
}

// The delete confirmation has to say what will be deleted and offer a way out.
// It answered nothing visible once, which is why this is pinned.
func TestTheDeleteFormNamesTheSong(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.get("/songs/traditional-amazing-grace/delete")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	res.mustContain("Amazing Grace", `action="/songs/traditional-amazing-grace/delete"`)
	res.mustShow("Cancelar")
}

// A competing edit is caught rather than silently overwriting somebody's work,
// and the reader keeps their text.
func TestSaveDetectsACompetingEdit(t *testing.T) {
	in := start(t, nil)
	in.seed()

	stale := in.editHash("traditional-amazing-grace")

	// Somebody else saves first.
	in.post("/songs/traditional-amazing-grace", url.Values{
		"text": {strings.Replace(amazingGrace, "sweet", "dulce", 1)},
		"hash": {stale},
	}).location()

	// The second save arrives with the hash from before that change.
	mine := strings.Replace(amazingGrace, "sweet", "MIO", 1)
	res := in.post("/songs/traditional-amazing-grace", url.Values{
		"text": {mine},
		"hash": {stale},
	})
	if res.Status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.Status)
	}
	res.mustShow("ha cambiado desde que abriste el editor")
	// The reader's work is still in the box, which is the whole point of saying
	// so instead of just refusing.
	res.mustContain("MIO")

	// And the file still holds the other person's version, not mine.
	raw := in.page("/songs/traditional-amazing-grace/raw")
	if strings.Contains(raw, "MIO") {
		t.Error("the conflicting save was written anyway")
	}
	if !strings.Contains(raw, "dulce") {
		t.Error("the first save was lost")
	}
}

// A malformed song comes back to the editor with the line to fix and the text
// intact, and nothing is written.
func TestAMalformedSongIsRefusedWithTheLine(t *testing.T) {
	in := start(t, nil)

	broken := "---\ntitle: T\ntitel: typo\n---\n\n## Estrofa 1\n[G]uno\n"
	res := in.post("/songs", url.Values{"text": {broken}})
	if res.Status == http.StatusSeeOther {
		t.Fatal("a song with an unknown field was accepted")
	}
	// The line number is the whole point of a raw editor, and the reader's own
	// text has to come back so they can fix it in place.
	res.mustShow("Línea 3")
	res.mustContain("titel")
	if got := in.songFiles(); len(got) != 0 {
		t.Errorf("a refused song was written anyway: %v", got)
	}
}

// An empty submission is a mistake, not a song.
func TestAnEmptySubmissionIsRefused(t *testing.T) {
	in := start(t, nil)

	res := in.post("/songs", url.Values{"text": {"   \n\n"}})
	if res.Status == http.StatusSeeOther {
		t.Error("an empty submission created something")
	}
	if got := in.songFiles(); len(got) != 0 {
		t.Errorf("an empty submission wrote %v", got)
	}
}

// Loading a file into the editor shows its text and writes nothing. An upload
// you cannot see before it lands is how a collection fills with files nobody
// meant to add.
func TestLoadingAFileOnlyFillsTheEditor(t *testing.T) {
	in := start(t, nil)

	res := in.upload("/songs/new", "file", map[string][]byte{"nueva.song": []byte(newSong)})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	res.mustContain("Una nueva", "Bm")
	if got := in.songFiles(); len(got) != 0 {
		t.Errorf("loading a file into the editor saved %v", got)
	}
}

// Export and import are the way a collection moves between machines, so the
// round trip has to be exact.
func TestExportThenImportIsFaithful(t *testing.T) {
	source := start(t, nil)
	source.seed()

	res := source.get("/export")
	if res.Status != http.StatusOK {
		t.Fatalf("export answered %d", res.Status)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	zr, err := zip.NewReader(bytes.NewReader(res.Raw), int64(len(res.Raw)))
	if err != nil {
		t.Fatalf("the export is not a readable zip: %v", err)
	}
	if len(zr.File) != 3 {
		var names []string
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		t.Fatalf("the export holds %d files (%v), want 3", len(zr.File), names)
	}

	// Into a fresh instance.
	target := start(t, nil)
	got := target.upload("/import", "files", map[string][]byte{"backup.zip": res.Raw})
	if got.Status != http.StatusOK {
		t.Fatalf("import answered %d", got.Status)
	}
	target.rescan()

	before, after := source.songFiles(), target.songFiles()
	if strings.Join(before, "|") != strings.Join(after, "|") {
		t.Errorf("the collection came back as %v, want %v", after, before)
	}
	target.get("/").mustContain("Amazing Grace", "Jazz", "Con cejilla")
}

// Importing a loose .song file works too, since that is what somebody has when
// a friend sends them one song.
func TestImportingASingleSongFile(t *testing.T) {
	in := start(t, nil)

	res := in.upload("/import", "files", map[string][]byte{"Una nueva.song": []byte(newSong)})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	in.rescan()
	in.get("/").mustContain("Una nueva")
}

// Import says what it did. A silent import leaves the reader unsure whether
// anything happened.
func TestImportReportsWhatHappened(t *testing.T) {
	in := start(t, nil)

	res := in.upload("/import", "files", map[string][]byte{
		"bien.song": []byte(newSong),
		"mal.song":  []byte("---\ntitle: T\ntitel: typo\n---\n"),
	})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	// One landed and one did not, and both facts have to be on the page.
	res.mustShow("mal.song")
	if !regexp.MustCompile(`(?i)1|una`).MatchString(res.Body) {
		t.Error("the report does not say how many songs were imported")
	}
}

// Import with no file chosen is a mistake to explain, not a crash.
func TestImportWithNoFileSaysSo(t *testing.T) {
	in := start(t, nil)

	res := in.upload("/import", "files", nil)
	if res.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.Status)
	}
	res.mustShow("No se ha seleccionado ningún archivo")
}

// Read-only mode is a promise about the filesystem, so it is tested against the
// filesystem: every route that writes must refuse, and nothing may appear.
func TestReadOnlyModeRefusesEveryWrite(t *testing.T) {
	in := start(t, map[string]string{"TABERNACLE_READ_ONLY": "true"})
	in.writeSong("Traditional/Amazing Grace.song", amazingGrace)

	writes := []struct {
		path string
		form url.Values
	}{
		{"/songs", url.Values{"text": {newSong}}},
		{"/songs/traditional-amazing-grace", url.Values{"text": {newSong}, "hash": {"x"}}},
		{"/songs/traditional-amazing-grace/delete", nil},
		{"/songs/traditional-amazing-grace/transpose", url.Values{"t": {"2"}}},
		{"/songs/traditional-amazing-grace/commit-transpose", url.Values{"t": {"2"}}},
		{"/reindex", nil},
	}
	for _, w := range writes {
		t.Run(w.path, func(t *testing.T) {
			res := in.post(w.path, w.form)
			if res.Status != http.StatusForbidden {
				t.Errorf("POST %s = %d, want 403", w.path, res.Status)
			}
		})
	}
	for _, path := range []string{"/import", "/songs/new"} {
		t.Run(path, func(t *testing.T) {
			res := in.upload(path, "files", map[string][]byte{"x.song": []byte(newSong)})
			if res.Status != http.StatusForbidden {
				t.Errorf("POST %s = %d, want 403", path, res.Status)
			}
		})
	}

	// The collection is exactly as it was, and reading still works.
	if got := in.songFiles(); len(got) != 1 {
		t.Errorf("read-only mode changed the collection: %v", got)
	}
	in.get("/").mustContain("Amazing Grace")
}

// Remembering a key is a preference, not an edit: it survives a reload and the
// file is untouched.
func TestRememberingAKeyDoesNotTouchTheFile(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.post("/songs/traditional-amazing-grace/transpose", url.Values{"t": {"2"}})
	if res.Status != http.StatusSeeOther && res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	in.get("/songs/traditional-amazing-grace").mustContain("A7")

	raw := in.page("/songs/traditional-amazing-grace/raw")
	if strings.Contains(raw, "A7") {
		t.Error("remembering a key rewrote the file")
	}
}

// Committing a transposition is the opposite: it is meant to change the file.
func TestCommittingATranspositionRewritesTheFile(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.post("/songs/traditional-amazing-grace/commit-transpose", url.Values{"t": {"2"}})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body: %.300s", res.Status, res.Body)
	}
	raw := in.page(strings.SplitN(res.Header.Get("Location"), "?", 2)[0] + "/raw")
	if !strings.Contains(raw, "A7") {
		t.Errorf("the file was not transposed:\n%s", raw)
	}
	if strings.Contains(raw, "G7") {
		t.Error("the old chords are still in the file")
	}
}
