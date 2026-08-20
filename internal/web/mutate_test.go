package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// post submits a form the way a same-origin browser would.
func (ts *testServer) post(t *testing.T, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

const pasted = `---
format: 1
title: Nueva Canción
artist: Alguien
key: Am
tags: [prueba]
---

## Estrofa 1
A[Am]quí van las [F]letras
`

func TestCreateFromPastedText(t *testing.T) {
	ts := newTestServer(t, nil)

	w := ts.post(t, "/songs", url.Values{"text": {pasted}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /songs = %d, want 303\n%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/songs/alguien-nueva-cancion") {
		t.Errorf("redirected to %q", loc)
	}

	// The file landed where the metadata says it should, not anywhere the
	// request could influence.
	path := filepath.Join(ts.songs, "Alguien", "Nueva Canción.song")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the song was not written to %s: %v", path, err)
	}
	if !strings.Contains(string(data), "title: Nueva Canción") {
		t.Errorf("file contents = %q", data)
	}
	// And it is immediately readable through the interface.
	body := ts.get(t, loc[:strings.Index(loc+"?", "?")]).Body.String()
	if !strings.Contains(body, "Nueva Canción") {
		t.Error("the new song does not render")
	}
}

// A paste that does not parse must come back with the text intact and the line
// to fix, which is the entire point of showing a raw editor.
func TestCreateRejectsBadTextAndKeepsIt(t *testing.T) {
	ts := newTestServer(t, nil)
	bad := "---\nformat: 1\ntitel: typo\n---\n\n## Estrofa\n[G]hola\n"

	w := ts.post(t, "/songs", url.Values{"text": {bad}})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST with bad text = %d, want 422", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "titel") {
		t.Error("the error message does not name the problem")
	}
	if !strings.Contains(body, "Línea") && !strings.Contains(body, "nea <code>3") {
		t.Errorf("the response does not point at the line:\n%s", body)
	}
	// The user's text must still be in the textarea, escaped.
	if !strings.Contains(body, "titel: typo") {
		t.Error("the submitted text was thrown away")
	}
	// Nothing was written.
	entries, _ := os.ReadDir(ts.songs)
	if len(entries) != 0 {
		t.Errorf("a rejected paste left %d entries on disk", len(entries))
	}
}

func TestEditAndSaveRaw(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	slug := "traditional-amazing-grace"

	form := ts.get(t, "/songs/"+slug+"/edit").Body.String()
	if !strings.Contains(form, "title: Amazing Grace") {
		t.Fatal("the editor does not show the file")
	}
	// The hash of the loaded version travels with the form, which is how a
	// competing edit is detected rather than clobbered.
	if !strings.Contains(form, `name="hash"`) {
		t.Fatal("the editor does not carry the version it loaded")
	}
	hash := between(form, `name="hash" value="`, `"`)
	if len(hash) != 64 {
		t.Fatalf("hash = %q", hash)
	}

	edited := strings.Replace(graceSong, "key: G", "key: A", 1)
	w := ts.post(t, "/songs/"+slug, url.Values{"text": {edited}, "hash": {hash}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("saving = %d, want 303\n%s", w.Code, w.Body.String())
	}
	onDisk, _ := os.ReadFile(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"))
	if !strings.Contains(string(onDisk), "key: A") {
		t.Error("the edit was not written")
	}
	// Saved bytes are the user's own, not re-formatted underneath them.
	if string(onDisk) != strings.ReplaceAll(edited, "\r\n", "\n") {
		t.Error("the saved file does not match what was typed")
	}
}

// With a periodic rescan and a phone left open on the edit form, a lost update
// is not hypothetical.
func TestSaveDetectsACompetingEdit(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	slug := "traditional-amazing-grace"
	form := ts.get(t, "/songs/"+slug+"/edit").Body.String()
	hash := between(form, `name="hash" value="`, `"`)

	// Somebody else edits the file in the meantime.
	other := strings.Replace(graceSong, "key: G", "key: E", 1)
	if err := os.WriteFile(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"),
		[]byte(other), 0o644); err != nil {
		t.Fatal(err)
	}

	mine := strings.Replace(graceSong, "key: G", "key: D", 1)
	w := ts.post(t, "/songs/"+slug, url.Values{"text": {mine}, "hash": {hash}})
	if w.Code != http.StatusConflict {
		t.Fatalf("saving over a competing edit = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ha cambiado") {
		t.Error("the conflict is not explained")
	}
	// Neither version is lost: theirs is on disk, mine is in the form.
	onDisk, _ := os.ReadFile(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"))
	if !strings.Contains(string(onDisk), "key: E") {
		t.Error("the other edit was overwritten")
	}
	if !strings.Contains(w.Body.String(), "key: D") {
		t.Error("my text was not given back to me")
	}
}

// A favourite is a tag in the file, which is what makes the index disposable.
func TestFavoriteWritesTheTagIntoTheFile(t *testing.T) {
	ts := newTestServer(t, map[string]string{
		"A/Song.song": "---\nformat: 1\ntitle: Song\nartist: A\n---\n\n## V\n[G]x\n",
	})
	slug := "a-song"
	path := filepath.Join(ts.songs, "A", "Song.song")

	if w := ts.post(t, "/songs/"+slug+"/favorite", url.Values{"on": {"1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("favouriting = %d\n%s", w.Code, w.Body.String())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "favorite") {
		t.Fatalf("the tag was not written into the file:\n%s", data)
	}
	// And it is searchable straight away.
	if !strings.Contains(ts.get(t, "/?fav=1").Body.String(), "Song") {
		t.Error("the favourite does not appear in the favourites filter")
	}

	if w := ts.post(t, "/songs/"+slug+"/favorite", url.Values{"on": {"0"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("unfavouriting = %d", w.Code)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "favorite") {
		t.Errorf("the tag was not removed:\n%s", data)
	}
}

func TestDeleteMovesToTrash(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	w := ts.post(t, "/songs/traditional-amazing-grace/delete", nil)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("deleting = %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song")); !os.IsNotExist(err) {
		t.Error("the file is still in the collection")
	}
	// It is recoverable, which is why this is a move and not an unlink.
	trash := filepath.Join(filepath.Dir(ts.songs), "trash")
	entries, err := os.ReadDir(trash)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the trash holds %d files (%v)", len(entries), err)
	}
	if ts.get(t, "/songs/traditional-amazing-grace").Code != http.StatusNotFound {
		t.Error("the deleted song still answers")
	}
}

func TestRememberAndCommitTranspose(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	slug := "traditional-amazing-grace"
	path := filepath.Join(ts.songs, "Traditional", "Amazing Grace.song")
	before, _ := os.ReadFile(path)

	// Remembering a key is database-only: the file must not move.
	if w := ts.post(t, "/songs/"+slug+"/transpose", url.Values{"t": {"2"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("remembering the key = %d", w.Code)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("remembering a key rewrote the file")
	}
	// And the song now opens in that key without a query parameter.
	if !strings.Contains(ts.get(t, "/songs/"+slug).Body.String(), ">A<") {
		t.Error("the remembered key is not applied when the song is opened")
	}

	// Committing is the deliberate counterpart, and does rewrite the file.
	if w := ts.post(t, "/songs/"+slug+"/commit-transpose", url.Values{"t": {"2"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("committing = %d", w.Code)
	}
	committed, _ := os.ReadFile(path)
	if !strings.Contains(string(committed), "key: A") {
		t.Errorf("the file was not rewritten in the new key:\n%s", committed)
	}
	// The remembered transposition is cleared, or the song would move twice.
	body := ts.get(t, "/songs/"+slug).Body.String()
	if strings.Contains(body, "Transportada") {
		t.Error("the song is still being transposed on top of the committed key")
	}
}

func TestReindexButton(t *testing.T) {
	ts := newTestServer(t, nil)
	// Drop a file in behind the application's back.
	dir := filepath.Join(ts.songs, "Later")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Song.song"),
		[]byte("---\nformat: 1\ntitle: Song\nartist: Later\n---\n\n## V\n[G]x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := ts.post(t, "/reindex", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("reindexing = %d", w.Code)
	}
	if !strings.Contains(ts.get(t, "/").Body.String(), "Song") {
		t.Error("the new file was not picked up")
	}
}

// With no accounts there are no credentials for SameSite to protect, so the
// origin check is the entire defence: a page on another site must not be able to
// change anything.
func TestWritesRefuseForeignRequests(t *testing.T) {
	writes := []struct {
		target string
		form   url.Values
	}{
		{"/songs", url.Values{"text": {pasted}}},
		{"/songs/traditional-amazing-grace", url.Values{"text": {graceSong}}},
		{"/songs/traditional-amazing-grace/favorite", url.Values{"on": {"1"}}},
		{"/songs/traditional-amazing-grace/delete", nil},
		{"/songs/traditional-amazing-grace/transpose", url.Values{"t": {"2"}}},
		{"/songs/traditional-amazing-grace/commit-transpose", url.Values{"t": {"2"}}},
		{"/reindex", nil},
	}
	headers := []struct {
		name   string
		host   string
		origin string
		fetch  string
		wantOK bool
	}{
		{"same origin", "localhost:8080", "http://localhost:8080", "same-origin", true},
		{"no origin, allowed host", "localhost:8080", "", "", true},
		{"foreign origin", "localhost:8080", "https://evil.example", "same-origin", false},
		{"cross site", "localhost:8080", "http://localhost:8080", "cross-site", false},
		{"same site but not same origin", "localhost:8080", "http://localhost:8080", "same-site", false},
		{"origin matches but host is foreign", "evil.example", "http://evil.example", "same-origin", false},
	}
	for _, h := range headers {
		for _, wr := range writes {
			t.Run(h.name+" "+wr.target, func(t *testing.T) {
				ts := newTestServer(t, basicSongs())
				body := ""
				if wr.form != nil {
					body = wr.form.Encode()
				}
				r := httptest.NewRequest(http.MethodPost, wr.target, strings.NewReader(body))
				r.Host = h.host
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if h.origin != "" {
					r.Header.Set("Origin", h.origin)
				}
				if h.fetch != "" {
					r.Header.Set("Sec-Fetch-Site", h.fetch)
				}
				w := httptest.NewRecorder()
				ts.h.ServeHTTP(w, r)

				if h.wantOK {
					if w.Code >= 400 {
						t.Errorf("a legitimate write got %d\n%s", w.Code, w.Body.String())
					}
					return
				}
				if w.Code < 400 {
					t.Errorf("a foreign write succeeded with %d", w.Code)
				}
				// And nothing changed.
				if _, err := os.Stat(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song")); err != nil {
					t.Error("the song was deleted by a refused request")
				}
			})
		}
	}
}

func TestReadOnlyModeRefusesEveryWrite(t *testing.T) {
	base := t.TempDir()
	ts := newTestServerWithEnv(t, basicSongs(), base, map[string]string{
		"TABERNACLE_READ_ONLY": "true",
	})
	for _, target := range []string{
		"/songs",
		"/songs/traditional-amazing-grace",
		"/songs/traditional-amazing-grace/favorite",
		"/songs/traditional-amazing-grace/delete",
		"/reindex",
	} {
		w := ts.post(t, target, url.Values{"text": {pasted}, "on": {"1"}})
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s in read-only mode = %d, want 403", target, w.Code)
		}
	}
	// Reading still works, and the write controls are not offered.
	body := ts.get(t, "/songs/traditional-amazing-grace").Body.String()
	if !strings.Contains(body, "Amazing Grace") {
		t.Error("reading is broken in read-only mode")
	}
	if strings.Contains(body, "Eliminar") || strings.Contains(body, "Editar archivo") {
		t.Error("read-only mode still offers the write controls")
	}
}

// A body larger than the limit must be refused rather than read into memory.
func TestOversizeFormIsRefused(t *testing.T) {
	ts := newTestServer(t, nil)
	huge := url.Values{"text": {strings.Repeat("x", maxFormBytes+1024)}}
	w := ts.post(t, "/songs", huge)
	if w.Code < 400 {
		t.Errorf("an oversize form was accepted with %d", w.Code)
	}
}

// Flash messages come from a fixed list, so a query string can never put text on
// the page.
func TestFlashMessagesAreNotEchoed(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/?msg=%3Cscript%3Ealert(1)%3C/script%3E").Body.String()
	if strings.Contains(body, "alert(1)") {
		t.Error("a message from the query string was rendered")
	}
	if !strings.Contains(ts.get(t, "/?msg=saved").Body.String(), "Cambios guardados") {
		t.Error("a known message code was not shown")
	}
	if strings.Contains(ts.get(t, "/?msg=nonsense").Body.String(), "nonsense") {
		t.Error("an unknown message code was echoed")
	}
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
