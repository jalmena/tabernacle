package web

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// post submits a form the way a same-origin browser would, carrying whatever
// session the harness holds.
func (ts *testServer) post(t *testing.T, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := ""
	if form != nil {
		body = form.Encode()
	}
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if ts.session != "" {
		r.Header.Set("Cookie", ts.session)
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// postAnon is the same submission with no session, for asking what a stranger can
// do.
func (ts *testServer) postAnon(t *testing.T, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	held := ts.session
	ts.session = ""
	defer func() { ts.session = held }()
	return ts.post(t, target, form)
}

const pasted = `---
title: Nueva Canción
artist: Alguien
key: Am
genre: prueba
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
	bad := "---\ntitel: typo\n---\n\n## Estrofa\n[G]hola\n"

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

// Revisar has to report what it did. The action always worked; what made it read
// as unimplemented was that a scan finding nothing produced no visible
// difference at all, so the button was indistinguishable from a dead one.
func TestReindexReportsWhatItDid(t *testing.T) {
	ts := newTestServer(t, nil)

	// Nothing to find: it must still say so.
	w := ts.post(t, "/reindex", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reindexing an empty collection = %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Nada ha cambiado") {
		t.Errorf("a scan that found nothing said nothing visible:\n%s", body)
	}

	// Drop a file in behind the application's back.
	dir := filepath.Join(ts.songs, "Later")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Song.song"),
		[]byte(pasted), 0o644); err != nil {
		t.Fatal(err)
	}

	w = ts.post(t, "/reindex", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reindexing = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Añadidas") {
		t.Errorf("the report does not say a song was added:\n%s", body)
	}
	if strings.Contains(body, "Nada ha cambiado") {
		t.Error("the report claims nothing changed after adding a song")
	}
	if !strings.Contains(ts.get(t, "/").Body.String(), "Nueva") {
		t.Error("the new file was not picked up")
	}
}

// Deleting asks first, and the question names the song so nobody removes the
// wrong one.
func TestDeleteAsksBeforeDoingIt(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	slug := "traditional-amazing-grace"

	form := ts.get(t, "/songs/"+slug+"/delete")
	if form.Code != http.StatusOK {
		t.Fatalf("the confirmation page = %d", form.Code)
	}
	body := form.Body.String()
	if !strings.Contains(body, "Amazing Grace") {
		t.Error("the question does not name the song")
	}
	if !strings.Contains(body, "papelera") {
		t.Error("the question does not say where the file goes")
	}
	if !strings.Contains(body, `action="/songs/`+slug+`/delete"`) {
		t.Error("the confirmation form does not post anywhere")
	}
	// Merely asking must not delete anything.
	if _, err := os.Stat(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song")); err != nil {
		t.Fatal("asking the question deleted the song")
	}
	// The song page links to the question rather than acting at once.
	songPage := ts.get(t, "/songs/"+slug).Body.String()
	if !strings.Contains(songPage, `href="/songs/`+slug+`/delete"`) {
		t.Error("the song page does not link to the confirmation")
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
		// A browser scopes neither an origin nor a cookie by hostname alone, so
		// another service on the same name is somebody else.
		{"same name, another port", "localhost:8080", "http://localhost:9000", "same-origin", false},
		{"same name, no port", "localhost:8080", "http://localhost", "same-origin", false},
		{"another address on the LAN", "localhost:8080", "http://192.168.1.50:8080", "same-origin", false},
		{"unparseable origin", "localhost:8080", "::::", "same-origin", false},

		// A browser that declines to name its origin. Firefox does this on every
		// POST when the referrer policy is restrictive, and every response here
		// carries no-referrer — so this is not an edge case, it is what a form
		// submission from Firefox looks like. Sec-Fetch-Site is the evidence
		// instead: page content cannot set it.
		{"null origin, same-origin", "localhost:8080", "null", "same-origin", true},
		{"null origin, typed address", "localhost:8080", "null", "none", true},
		{"null origin, cross-site", "localhost:8080", "null", "cross-site", false},
		// And with no Sec-Fetch-Site either, the allowlisted Host is all there is.
		{"null origin, nothing else", "localhost:8080", "null", "", true},
		{"null origin, foreign host", "evil.example", "null", "", false},
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

// A notice comes from a fixed list, and it travels in a cookie rather than in the
// address, so nothing a browser sends can put text on the page and nothing is
// left in the history or in a shared link.
func TestANoticeIsShownOnceAndNeverEchoed(t *testing.T) {
	ts := newTestServer(t, basicSongs())

	// Saving a song redirects with a notice, and the address it redirects to
	// carries no trace of it.
	w := ts.post(t, "/songs", url.Values{"text": {pasted}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("creating a song = %d", w.Code)
	}
	if loc := w.Header().Get("Location"); strings.Contains(loc, "msg") {
		t.Errorf("the redirect went to %q, which names the notice", loc)
	}
	ts.carry(w)

	// Shown once...
	first := ts.get(t, "/")
	if !strings.Contains(first.Body.String(), "Canción guardada") {
		t.Error("the notice was not shown")
	}
	if !strings.Contains(first.Body.String(), "data-dismiss") {
		t.Error("the notice cannot be closed")
	}
	ts.carry(first)

	// ...and not again on a reload.
	if strings.Contains(ts.get(t, "/").Body.String(), "Canción guardada") {
		t.Error("the notice came back on the next page")
	}
}

// Whatever a browser puts in that cookie, only a code from the fixed list can
// reach the page.
func TestANoticeCannotBeForged(t *testing.T) {
	ts := newTestServer(t, basicSongs())

	for _, value := range []string{
		"<script>alert(1)</script>", "nonsense", "", "saved<b>",
	} {
		ts.session = flashCookie + "=" + url.QueryEscape(value)
		body := ts.get(t, "/").Body.String()
		if strings.Contains(body, "alert(1)") || strings.Contains(body, "nonsense") {
			t.Errorf("the cookie value %q reached the page", value)
		}
	}
	// And a real code does work, so the test above is not passing vacuously.
	ts.session = flashCookie + "=saved"
	if !strings.Contains(ts.get(t, "/").Body.String(), "Cambios guardados") {
		t.Error("a known code was not shown")
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

// uploadZip posts a multipart form with one file, as a browser would.
func (ts *testServer) upload(t *testing.T, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("files", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/import", &body)
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// uploadField posts one file under a chosen field name, which is how the editor
// load and the bulk import differ.
func (ts *testServer) uploadField(t *testing.T, field, target, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if filename != "" {
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, target, &body)
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// uploadForeign posts a file as another site would.
func (ts *testServer) uploadForeign(t *testing.T, target, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write(content)
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, target, &body)
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

func contextTODO() context.Context { return context.Background() }

func TestImportSingleUpload(t *testing.T) {
	ts := newTestServer(t, nil)
	w := ts.upload(t, "anything.song", []byte(pasted))
	if w.Code != http.StatusOK {
		t.Fatalf("upload = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Nueva Canción") {
		t.Errorf("the report does not name the imported song:\n%s", body)
	}
	// The destination came from the metadata, not from the upload's filename.
	if _, err := os.Stat(filepath.Join(ts.songs, "Alguien", "Nueva Canción.song")); err != nil {
		t.Errorf("the song is not where its metadata says: %v", err)
	}
	if !strings.Contains(ts.get(t, "/").Body.String(), "Nueva Canción") {
		t.Error("the imported song is not in the library")
	}
}

func TestImportZipThroughTheWeb(t *testing.T) {
	ts := newTestServer(t, nil)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct{ name, body string }{
		{"a/Good.song", pasted},
		{"../evil.song", pasted},                // zip slip
		{"notes.txt", "not a song"},             // wrong kind
		{"bad.song", "---\ntitel: typo\n---\n"}, // will not parse
	} {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(0o644)
		f, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(e.body))
	}
	zw.Close()

	w := ts.upload(t, "backup.zip", buf.Bytes())
	if w.Code != http.StatusOK {
		t.Fatalf("upload = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"añadida", "Con errores", "Omitidos"} {
		if !strings.Contains(body, want) {
			t.Errorf("the report is missing the %q section:\n%s", want, body)
		}
	}
	// Exactly one song, and nothing outside the collection.
	var count int
	filepath.WalkDir(ts.songs, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			count++
		}
		return nil
	})
	if count != 1 {
		t.Errorf("%d files were written, want 1", count)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(ts.songs), "evil.song")); err == nil {
		t.Fatal("the zip slip entry escaped the collection")
	}
}

// A zip renamed to .song must still be treated as a zip: a browser's filename is
// not evidence of anything.
func TestImportDetectsAZipByItsContent(t *testing.T) {
	ts := newTestServer(t, nil)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "x.song", Method: zip.Deflate}
	h.SetMode(0o644)
	f, _ := zw.CreateHeader(h)
	f.Write([]byte(pasted))
	zw.Close()

	w := ts.upload(t, "actually-a-zip.song", buf.Bytes())
	if w.Code != http.StatusOK {
		t.Fatalf("upload = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Nueva Canción") {
		t.Errorf("the archive was not unpacked:\n%s", w.Body.String())
	}
}

func TestImportWithNoFileSelected(t *testing.T) {
	ts := newTestServer(t, nil)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/import", &body)
	r.Host = "localhost"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "http://localhost")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("an empty upload = %d, want 400", w.Code)
	}
}

func TestImportRefusesForeignOrigin(t *testing.T) {
	ts := newTestServer(t, nil)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("files", "x.song")
	fw.Write([]byte(pasted))
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", &body)
	r.Host = "localhost"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("a cross-origin upload = %d, want 403", w.Code)
	}
	entries, _ := os.ReadDir(ts.songs)
	if len(entries) != 0 {
		t.Error("the refused upload wrote something")
	}
}

func TestExportDownload(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	w := ts.get(t, "/export")
	if w.Code != http.StatusOK {
		t.Fatalf("export = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "tabernacle-") {
		t.Errorf("content disposition = %q", cd)
	}

	data := w.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the export is not a valid archive: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("the archive holds %d files, want 1", len(zr.File))
	}
	if zr.File[0].Name != "songs/Traditional/Amazing Grace.song" {
		t.Errorf("entry name = %q, want the collection's own layout", zr.File[0].Name)
	}
	// The bytes are the file's, so restoring is unzipping.
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want, _ := os.ReadFile(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"))
	if string(got) != string(want) {
		t.Error("the exported bytes differ from the file on disk")
	}
}

// Export is a read, so it stays available even when writes are refused.
func TestExportWorksInReadOnlyMode(t *testing.T) {
	ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
		"TABERNACLE_READ_ONLY": "true",
	})
	if code := ts.get(t, "/export").Code; code != http.StatusOK {
		t.Errorf("export in read-only mode = %d", code)
	}
	// Import is a write, so it is not.
	if code := ts.upload(t, "x.song", []byte(pasted)).Code; code != http.StatusForbidden {
		t.Errorf("import in read-only mode = %d, want 403", code)
	}
}

// originAllowed is the whole of "may this page submit here", so its two
// configured escape hatches are checked directly: a reverse proxy's public name,
// and a name the operator listed on purpose.
func TestOriginAllowedHonoursConfiguration(t *testing.T) {
	ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
		"TABERNACLE_BASE_URL":      "https://songs.example.com",
		"TABERNACLE_ALLOWED_HOSTS": "songs.example.com,nas.local",
	})

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		// The proxy rewrites Host; the browser's origin is the public name.
		{"the configured base URL", "https://songs.example.com", "tabernacle:8080", true},
		{"the exact host we were addressed as", "http://tabernacle:8080", "tabernacle:8080", true},
		{"an allowlisted name on our port", "http://nas.local:8080", "nas.local:8080", true},

		// The port has to agree even for a name that was listed on purpose.
		{"an allowlisted name on another port", "http://nas.local:9000", "nas.local:8080", false},
		{"a name that was never listed", "http://other.local:8080", "nas.local:8080", false},
		{"the base URL's name on another port", "https://songs.example.com:9000", "tabernacle:8080", false},
		{"no host at all", "null", "nas.local:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ts.srv.originAllowed(tt.origin, tt.host); got != tt.want {
				t.Errorf("originAllowed(%q, %q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}
