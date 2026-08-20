package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/songfs"
)

type testServer struct {
	h     http.Handler
	lib   *library.Library
	idx   *index.Index
	songs string
}

func newTestServer(t *testing.T, songs map[string]string) *testServer {
	t.Helper()
	return newTestServerWithEnv(t, songs, t.TempDir(), nil)
}

// newTestServerWithEnv builds a server with extra configuration, so a test can
// exercise something like read-only mode without a second harness.
func newTestServerWithEnv(t *testing.T, songs map[string]string, base string, extra map[string]string) *testServer {
	t.Helper()
	songsDir := filepath.Join(base, "songs")
	trashDir := filepath.Join(base, "trash")

	for rel, content := range songs {
		full := filepath.Join(songsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := songfs.Open(songsDir, trashDir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Open(context.Background(), filepath.Join(base, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); idx.Close() })

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	lib := library.New(store, idx, quiet)
	if _, err := lib.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"TABERNACLE_DATA_DIR":  base,
		"TABERNACLE_SONGS_DIR": songsDir,
		"TABERNACLE_TRASH_DIR": trashDir,
	}
	for k, v := range extra {
		env[k] = v
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(lib, cfg, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return &testServer{h: srv.Handler(), lib: lib, idx: idx, songs: songsDir}
}

// get performs a request with a Host the default policy accepts.
func (ts *testServer) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = "localhost:8080"
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

const graceSong = `---
format: 1
title: Amazing Grace
artist: Traditional
key: G
capo: 2
tags: [hymn, favorite]
notes: Public domain.
---

## Verse 1
A[G]mazing grace how [C]sweet the [G]sound
That saved a [Em]wretch like [D]me [x2]

I once was lost but now am found

## Solo
` + "```tab" + `
e|---0---3---|
` + "```" + `
`

func basicSongs() map[string]string {
	return map[string]string{"Traditional/Amazing Grace.song": graceSong}
}

func TestRoutes(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	tests := []struct {
		target string
		want   int
	}{
		{"/", http.StatusOK},
		{"/songs/traditional-amazing-grace", http.StatusOK},
		{"/songs/traditional-amazing-grace/sheet", http.StatusOK},
		{"/songs/traditional-amazing-grace/print", http.StatusOK},
		{"/songs/traditional-amazing-grace/raw", http.StatusOK},
		{"/songs/traditional-amazing-grace?t=2&n=solfege&capo=sounding", http.StatusOK},
		{"/tags", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/robots.txt", http.StatusOK},
		{"/?q=grace", http.StatusOK},
		{"/?tag=hymn", http.StatusOK},
		{"/?chord=G", http.StatusOK},
		{"/?fav=1", http.StatusOK},
		{"/?sort=artist", http.StatusOK},
		{"/?page=99", http.StatusOK},
		{"/songs/does-not-exist", http.StatusNotFound},
		{"/songs/does-not-exist/raw", http.StatusNotFound},
		{"/nope", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := ts.get(t, tt.target).Code; got != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.target, got, tt.want)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, target := range []string{"/tags", "/songs/traditional-amazing-grace/raw", "/healthz"} {
		r := httptest.NewRequest(http.MethodPost, target, nil)
		r.Host = "localhost"
		w := httptest.NewRecorder()
		ts.h.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", target, w.Code)
		}
	}
}

// Nothing a user can put in a song file may become markup or script.
func TestNoScriptEscapesFromASong(t *testing.T) {
	// Every value here has to be markup and still be a legal .song file, or the
	// test would only be exercising the broken-file page.
	hostile := `---
format: 1
title: <script>alert('title')</script>
artist: x"><img src=x onerror=alert('artist')>
key: G
tuning: <svg onload=alert('tuning')>
tags: ["<script>alert('tag')</script>", "javascript:alert(1)"]
notes: |-
  <script>alert('notes')</script>
  second line
---

## <svg onload=alert('label')>
A[G]mazing <script>alert('lyric')</script> grace
Text with [<b>x2</b>] an annotation

## Solo
` + "```tab" + `
</pre><script>alert('tab')</script>
` + "```" + `
`
	ts := newTestServer(t, map[string]string{"Evil/Song.song": hostile})

	for _, target := range []string{
		"/",
		"/?q=alert",
		"/tags",
		"/songs/evil-song",
		"/songs/evil-song/sheet",
		"/songs/evil-song/print",
	} {
		t.Run(target, func(t *testing.T) {
			w := ts.get(t, target)
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", target, w.Code)
			}
			body := w.Body.String()

			// The dangerous forms must not be present.
			for _, bad := range []string{
				"<script>alert",
				"<img src=x onerror",
				"<svg onload",
				"</pre><script>",
				"<b>x2</b>",
			} {
				if strings.Contains(body, bad) {
					t.Errorf("response contains live markup %q", bad)
				}
			}
			// The application's own script tag is the only one allowed.
			for _, line := range strings.Split(body, "\n") {
				if strings.Contains(line, "<script") && !strings.Contains(line, "/app.js") {
					t.Errorf("unexpected script tag: %s", strings.TrimSpace(line))
				}
			}
		})
	}

	// The fixture must have parsed, or this test would only be checking the
	// broken-file page and would prove nothing about escaping.
	body := ts.get(t, "/songs/evil-song").Body.String()
	if strings.Contains(body, "error de sintaxis") {
		t.Fatal("the hostile fixture did not parse, so nothing was escaped")
	}
	// And the escaped form is present, so the test is not passing merely because
	// the content went missing.
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("the song text is missing; it should be present but escaped")
	}
	if !strings.Contains(body, "&lt;b&gt;x2&lt;/b&gt;") {
		t.Error("the annotation's markup was not escaped as text")
	}
}

// A lyric containing markup must survive a search excerpt as text.
func TestSearchExcerptEscapesMarkup(t *testing.T) {
	hostile := "---\nformat: 1\ntitle: Sneaky\n---\n\n## Verse 1\n" +
		"a line with <script>alert(1)</script> and the word wretch in it\n"
	ts := newTestServer(t, map[string]string{"A/Sneaky.song": hostile})

	body := ts.get(t, "/?q=wretch").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("the search excerpt emitted the lyric's markup live")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("the excerpt does not contain the escaped lyric")
	}
	// The highlight comes from the template, so it is real markup.
	if !strings.Contains(body, "<mark>") {
		t.Error("the matched term is not highlighted")
	}
}

// The Host allowlist is the defence against DNS rebinding, so it has to hold
// for every route including the health check.
func TestHostAllowlist(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	tests := []struct {
		host string
		want int
	}{
		{"localhost:8080", http.StatusOK},
		{"127.0.0.1:8080", http.StatusOK},
		{"192.168.1.50:8080", http.StatusOK},
		{"nas", http.StatusOK},
		{"tabernacle.local", http.StatusOK},
		{"box.home.arpa", http.StatusOK},
		// A public name pointed at a private address is exactly the shape a
		// rebinding attack takes.
		{"attacker.com", http.StatusMisdirectedRequest},
		{"tabernacle.attacker.com", http.StatusMisdirectedRequest},
		{"8.8.8.8", http.StatusMisdirectedRequest},
	}
	for _, tt := range tests {
		for _, target := range []string{"/", "/songs/traditional-amazing-grace", "/healthz"} {
			r := httptest.NewRequest(http.MethodGet, target, nil)
			r.Host = tt.host
			w := httptest.NewRecorder()
			ts.h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Errorf("Host %q on %s = %d, want %d", tt.host, target, w.Code, tt.want)
			}
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	h := ts.get(t, "/").Header()

	csp := h.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("no content security policy")
	}
	// No inline script or style anywhere, so the policy needs no escape hatch.
	for _, forbidden := range []string{"unsafe-inline", "unsafe-eval", "*"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("policy contains %q: %s", forbidden, csp)
		}
	}
	for _, want := range []string{"script-src 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy is missing %q: %s", want, csp)
		}
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff is not set")
	}
	if h.Get("Referrer-Policy") != "no-referrer" {
		t.Error("referrer policy is not set")
	}
}

func TestTranspositionAndNotationThroughTheURL(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	chordsOf := func(target string) string {
		body := ts.get(t, target).Body.String()
		var out []string
		for _, part := range strings.Split(body, `class="chordref"`)[1:] {
			if i := strings.Index(part, ">"); i >= 0 {
				rest := part[i+1:]
				if j := strings.Index(rest, "<"); j >= 0 {
					out = append(out, rest[:j])
				}
			}
		}
		return strings.Join(out, " ")
	}
	tests := []struct{ target, want string }{
		{"/songs/traditional-amazing-grace", "G C Em D"},
		{"/songs/traditional-amazing-grace?t=2", "A D F#m E"},
		{"/songs/traditional-amazing-grace?t=-1", "F# B D#m C#"},
		{"/songs/traditional-amazing-grace?n=solfege", "Sol Do Mim Re"},
		{"/songs/traditional-amazing-grace?t=2&n=solfege", "La Re Fa#m Mi"},
		// An octave is the same sheet.
		{"/songs/traditional-amazing-grace?t=12", "G C Em D"},
		// Nonsense degrades to the song rather than an error.
		{"/songs/traditional-amazing-grace?t=abc", "G C Em D"},
		// Capo 2, so what actually sounds is a tone higher.
		{"/songs/traditional-amazing-grace?capo=sounding", "A D F#m E"},
	}
	for _, tt := range tests {
		if got := chordsOf(tt.target); got != tt.want {
			t.Errorf("GET %s chords = %q, want %q", tt.target, got, tt.want)
		}
	}
}

// The file on disk must never change because somebody looked at a transposition.
func TestTransposingDoesNotTouchTheFile(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	path := filepath.Join(ts.songs, "Traditional", "Amazing Grace.song")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	statBefore, _ := os.Stat(path)

	for _, target := range []string{
		"/songs/traditional-amazing-grace?t=5",
		"/songs/traditional-amazing-grace?t=-3&n=solfege",
		"/songs/traditional-amazing-grace?capo=sounding",
		"/songs/traditional-amazing-grace/print?t=7",
	} {
		if code := ts.get(t, target).Code; code != http.StatusOK {
			t.Fatalf("GET %s = %d", target, code)
		}
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the .song file changed while the song was only being viewed")
	}
	statAfter, _ := os.Stat(path)
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Error("the file's modification time changed while only viewing")
	}
}

// The fragment must carry exactly the two regions the script swaps, so the
// scroll position is preserved rather than the page being replaced.
func TestSheetFragment(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/songs/traditional-amazing-grace/sheet?t=1").Body.String()

	for _, want := range []string{`id="sheet"`, `id="controls"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the fragment is missing %s", want)
		}
	}
	// It is a fragment, not a page.
	for _, unwanted := range []string{"<!DOCTYPE", "<html", "<head", "class=\"top\""} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the fragment contains %q; it should be only the swappable region", unwanted)
		}
	}
}

// Every control has to be a real link, or the application stops working without
// JavaScript.
func TestControlsAreRealLinks(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/songs/traditional-amazing-grace?t=1").Body.String()
	for _, want := range []string{
		`data-sheet="down" href="/songs/traditional-amazing-grace"`,
		`data-sheet="up" href="/songs/traditional-amazing-grace?t=2"`,
		`data-sheet="reset" href="/songs/traditional-amazing-grace"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected a plain link: %s", want)
		}
	}
	// The notation toggle and the print link too.
	if !strings.Contains(body, `href="/songs/traditional-amazing-grace?n=solfege&amp;t=1"`) {
		t.Error("the notation toggle is not a plain link carrying the current transposition")
	}
	// The default notation is never named in a URL, so a shared link stays clean.
	if strings.Contains(body, "n=english") {
		t.Error("links carry ?n=english even though that is the default")
	}
}

func TestPrintViewOmitsInteractiveParts(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/songs/traditional-amazing-grace/print").Body.String()

	for _, unwanted := range []string{`id="controls"`, "app.js", `class="search"`, `class="top"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the print view contains %q", unwanted)
		}
	}
	// But everything a player needs is there.
	for _, want := range []string{"Amazing Grace", "Traditional", "chordref", "Cejilla", "class=\"tab\""} {
		if !strings.Contains(body, want) {
			t.Errorf("the print view is missing %q", want)
		}
	}
}

func TestRawDownload(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	w := ts.get(t, "/songs/traditional-amazing-grace/raw")

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type = %q, want text/plain so a browser never runs it", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "Amazing Grace.song") {
		t.Errorf("content disposition = %q", cd)
	}
	// Byte-for-byte what is on disk: this is the escape hatch that keeps the
	// format honest.
	onDisk, err := os.ReadFile(filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"))
	if err != nil {
		t.Fatal(err)
	}
	if w.Body.String() != string(onDisk) {
		t.Error("the download does not match the file on disk")
	}
}

// A renamed file must redirect, not 404, or every bookmark breaks.
func TestRenamedSongRedirects(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	if err := os.MkdirAll(filepath.Join(ts.songs, "New Name"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"),
		filepath.Join(ts.songs, "New Name", "Amazing Grace.song")); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.lib.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	w := ts.get(t, "/songs/traditional-amazing-grace?t=2")
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("GET the old URL = %d, want 301", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/songs/new-name-amazing-grace") {
		t.Errorf("redirected to %q", loc)
	}
	// The transposition must survive the redirect, or the reader loses their key.
	if !strings.Contains(loc, "t=2") {
		t.Errorf("the redirect dropped the query: %q", loc)
	}
}

func TestBrokenSongShowsTheLineToFix(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	if err := os.WriteFile(
		filepath.Join(ts.songs, "Traditional", "Amazing Grace.song"),
		[]byte("---\nformat: 1\ntitel: typo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.lib.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := ts.get(t, "/songs/traditional-amazing-grace").Body.String()
	if !strings.Contains(body, "error de sintaxis") {
		t.Error("the page does not say the file is broken")
	}
	if !strings.Contains(body, "línea") {
		t.Error("the page does not name the line to fix")
	}
	// The song is still listed, not vanished.
	if !strings.Contains(ts.get(t, "/").Body.String(), "Amazing Grace") {
		t.Error("the broken song disappeared from the library")
	}
}

func TestEmptyLibraryExplainsWhatToDo(t *testing.T) {
	ts := newTestServer(t, nil)
	body := ts.get(t, "/").Body.String()
	if !strings.Contains(body, "vacío") {
		t.Error("an empty library does not say so")
	}
	// It names the directory to drop files into, which is the one thing a new
	// user needs to know.
	if !strings.Contains(body, "Artista/Título.song") {
		t.Error("the empty state does not explain where songs go")
	}
	// A search that found nothing is a different message.
	if strings.Contains(ts.get(t, "/?q=nothing").Body.String(), "vacío") {
		t.Error("a fruitless search claims the library is empty")
	}
}

func TestStaticAssets(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	page := ts.get(t, "/").Body.String()

	i := strings.Index(page, "/static/")
	if i < 0 {
		t.Fatal("the page does not reference a stylesheet")
	}
	url := page[i:]
	url = url[:strings.Index(url, `"`)]

	w := ts.get(t, url)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", url, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content type = %q", ct)
	}
	// The URL carries a content hash, so it can be cached forever.
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("cache control = %q, want it immutable", cc)
	}
	// A stale hash still serves the file, so old HTML keeps working, but is not
	// cached as if it were current.
	stale := ts.get(t, "/static/0123456789ab/app.css")
	if stale.Code != http.StatusOK {
		t.Errorf("a stale asset URL = %d, want it still served", stale.Code)
	}
	if strings.Contains(stale.Header().Get("Cache-Control"), "immutable") {
		t.Error("a stale asset URL was marked immutable")
	}
	if ts.get(t, "/static/x/../../etc/passwd").Code == http.StatusOK {
		t.Error("the asset handler served something outside the embedded files")
	}
}

func TestRobotsDisallowsEverything(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/robots.txt").Body.String()
	if !strings.Contains(body, "Disallow: /") {
		t.Errorf("robots.txt = %q", body)
	}
}

func TestHealthReportsCounts(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	body := ts.get(t, "/healthz").Body.String()
	for _, want := range []string{`"status":"ok"`, `"songs":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("health = %q, want it to contain %q", body, want)
		}
	}
}

func TestContentDisposition(t *testing.T) {
	tests := []struct{ in, wantASCII, wantExt string }{
		{"Amazing Grace.song", `filename="Amazing Grace.song"`, "Amazing%20Grace.song"},
		{"Y nos dieron las diez.song", `filename="Y nos dieron las diez.song"`, "diez.song"},
		{"Corazón.song", `filename="Coraz_n.song"`, "Coraz%C3%B3n.song"},
		// A quote or a newline here is how this header gets split.
		{"a\"b\nc.song", `filename="abc.song"`, "a%22b%0Ac.song"},
		{"", `filename="song.song"`, ""},
	}
	for _, tt := range tests {
		got := contentDisposition(tt.in)
		if !strings.Contains(got, tt.wantASCII) {
			t.Errorf("contentDisposition(%q) = %q, want it to contain %q", tt.in, got, tt.wantASCII)
		}
		if tt.wantExt != "" && !strings.Contains(got, tt.wantExt) {
			t.Errorf("contentDisposition(%q) = %q, want it to contain %q", tt.in, got, tt.wantExt)
		}
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("contentDisposition(%q) produced a header with a newline", tt.in)
		}
	}
}

func TestEncodeQueryIsStable(t *testing.T) {
	// Keys are sorted, so the same parameters always give the same URL and the
	// client-side cache does not store one page twice.
	a := encodeQuery(map[string][]string{"t": {"2"}, "n": {"solfege"}})
	b := encodeQuery(map[string][]string{"n": {"solfege"}, "t": {"2"}})
	if a != b {
		t.Errorf("query order is not stable: %q and %q", a, b)
	}
	if a != "?n=solfege&t=2" {
		t.Errorf("encodeQuery = %q", a)
	}
	if encodeQuery(nil) != "" {
		t.Error("an empty query should produce no question mark")
	}
	// Values are escaped, not pasted.
	if got := encodeQuery(map[string][]string{"chord": {"F#m/A"}}); got != "?chord=F%23m%2FA" {
		t.Errorf("encodeQuery did not escape the value: %q", got)
	}
}
