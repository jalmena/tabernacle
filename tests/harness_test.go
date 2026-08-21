// Package tests drives a real Tabernacle over HTTP.
//
// Every test here starts the application the way the binary does — through
// app.Build, against a real directory of .song files and a real SQLite index —
// and then talks to it with an HTTP client. Nothing is faked or stubbed.
//
// The unit tests beside each package answer whether a part is correct. These
// answer a different question: whether the thing a reader opens in a browser
// works, end to end, including the wiring, the routing, the templates, the
// escaping and the files on disk. A bug that lives in the seams between correct
// parts can only be caught here.
package tests

import (
	"bytes"
	"context"
	"html"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/app"
	"github.com/jalmena/tabernacle/internal/config"
)

// instance is one running Tabernacle, with the directory it serves.
type instance struct {
	t     *testing.T
	env   map[string]string
	dir   string // the data directory
	songs string // the songs directory inside it
	url   string // where it is listening

	app *app.App
	srv *httptest.Server
}

// start boots an instance on a fresh temporary directory.
//
// The environment is the real one: config.Load reads these exactly as it reads
// the process environment, so a test can exercise any supported setting without
// a special path through the code.
func start(t *testing.T, env map[string]string) *instance {
	t.Helper()

	dir := t.TempDir()
	full := map[string]string{"TABERNACLE_DATA_DIR": dir}
	for k, v := range env {
		full[k] = v
	}

	in := &instance{
		t:     t,
		env:   full,
		dir:   dir,
		songs: filepath.Join(dir, "songs"),
	}
	in.boot()
	t.Cleanup(in.stop)
	return in
}

func (in *instance) boot() {
	in.t.Helper()

	cfg, err := config.Load(func(k string) string { return in.env[k] })
	if err != nil {
		in.t.Fatalf("config: %v", err)
	}

	// Discarded rather than captured: these tests judge what a reader sees, and
	// a test that passes because of a log line is testing the wrong surface.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	a, err := app.Build(context.Background(), cfg, log)
	if err != nil {
		in.t.Fatalf("build: %v", err)
	}
	in.app = a
	in.srv = httptest.NewServer(a.Handler)
	in.url = in.srv.URL
}

func (in *instance) stop() {
	if in.srv != nil {
		in.srv.Close()
		in.srv = nil
	}
	if in.app != nil {
		if err := in.app.Close(); err != nil {
			in.t.Errorf("close: %v", err)
		}
		in.app = nil
	}
}

// restart stops the instance and brings it up again on the same directory.
//
// This is what a container restart does, and it is the only honest way to test
// that the collection survives one.
func (in *instance) restart() {
	in.t.Helper()
	in.stop()
	in.boot()
}

// client refuses to follow redirects, so a test can see the 303 and its
// destination rather than only the page at the end of the chain.
func (in *instance) client() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// response is a finished exchange, with the body already read.
type response struct {
	t      *testing.T
	Status int
	Body   string
	Header http.Header
	Raw    []byte
}

func (in *instance) do(req *http.Request) *response {
	in.t.Helper()
	res, err := in.client().Do(req)
	if err != nil {
		in.t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		in.t.Fatalf("reading the body of %s: %v", req.URL, err)
	}
	return &response{t: in.t, Status: res.StatusCode, Body: string(raw), Header: res.Header, Raw: raw}
}

// get fetches a path. Redirects are not followed.
func (in *instance) get(path string) *response {
	in.t.Helper()
	req, err := http.NewRequest(http.MethodGet, in.url+path, nil)
	if err != nil {
		in.t.Fatalf("building a request for %s: %v", path, err)
	}
	return in.do(req)
}

// page fetches a path that must answer 200, and returns the HTML.
func (in *instance) page(path string) string {
	in.t.Helper()
	res := in.get(path)
	if res.Status != http.StatusOK {
		in.t.Fatalf("GET %s = %d, want 200", path, res.Status)
	}
	return res.Body
}

// post submits a form the way the page's own form does: same origin, and the
// header a browser would send with it.
func (in *instance) post(path string, form url.Values) *response {
	in.t.Helper()
	req, err := http.NewRequest(http.MethodPost, in.url+path, strings.NewReader(form.Encode()))
	if err != nil {
		in.t.Fatalf("building a request for %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", in.url)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return in.do(req)
}

// upload submits one or more files as multipart, under the given field name.
func (in *instance) upload(path, field string, files map[string][]byte) *response {
	in.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := mw.CreateFormFile(field, name)
		if err != nil {
			in.t.Fatalf("multipart: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			in.t.Fatalf("multipart: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		in.t.Fatalf("multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, in.url+path, &buf)
	if err != nil {
		in.t.Fatalf("building a request for %s: %v", path, err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", in.url)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return in.do(req)
}

// writeSong puts a file into the collection behind the application's back, the
// way copying a file into the volume does, and then asks for a scan.
func (in *instance) writeSong(rel, content string) {
	in.t.Helper()
	path := filepath.Join(in.songs, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		in.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		in.t.Fatal(err)
	}
	in.rescan()
}

// rescan runs the scan the periodic timer would have run.
func (in *instance) rescan() {
	in.t.Helper()
	if _, err := in.app.Library.Sync(context.Background()); err != nil {
		in.t.Fatalf("scan: %v", err)
	}
}

// songFiles lists the collection as it exists on disk, relative to the songs
// directory, so a test can assert about the source of truth rather than about
// the index's opinion of it.
func (in *instance) songFiles() []string {
	in.t.Helper()
	var out []string
	err := filepath.WalkDir(in.songs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".song") {
			return nil
		}
		rel, err := filepath.Rel(in.songs, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		in.t.Fatalf("walking the collection: %v", err)
	}
	return out
}

// location is the redirect target of a response that must be a 303.
func (r *response) location() string {
	r.t.Helper()
	if r.Status != http.StatusSeeOther {
		r.t.Fatalf("status = %d, want 303; body was %.300s", r.Status, r.Body)
	}
	loc := r.Header.Get("Location")
	if loc == "" {
		r.t.Fatal("a 303 with no Location")
	}
	return loc
}

func (r *response) mustContain(what ...string) {
	r.t.Helper()
	for _, w := range what {
		if !strings.Contains(r.Body, w) {
			r.t.Errorf("the response does not contain %q", w)
		}
	}
}

// mustShow asserts that text appears as a reader would see it, with HTML
// entities resolved.
//
// Go's templates escape more than the minimum — a plain "+" comes out as
// "&#43;" — so an assertion about visible text has to look at the text. The byte
// comparisons above stay as they are, because the tests that care about markup
// injection care about exactly those bytes.
func (r *response) mustShow(what ...string) {
	r.t.Helper()
	text := visibleText(r.Body)
	for _, w := range what {
		if !strings.Contains(text, w) {
			r.t.Errorf("the page does not show %q", w)
		}
	}
}

// visibleText reduces a page to what a reader reads: no tags, no entities, and
// runs of whitespace collapsed.
//
// Assertions about wording need this because the markup falls where it likes.
// "Línea <code>3</code>: unknown field" reads as one sentence and has to be
// testable as one, or every such test ends up pinned to a template's tag soup.
func visibleText(body string) string {
	text := tagPattern.ReplaceAllString(body, " ")
	text = html.UnescapeString(text)
	return strings.Join(strings.Fields(text), " ")
}

var tagPattern = regexp.MustCompile(`(?s)<(script|style)\b.*?</(script|style)>|<[^>]*>`)

func (r *response) mustNotContain(what ...string) {
	r.t.Helper()
	for _, w := range what {
		if strings.Contains(r.Body, w) {
			r.t.Errorf("the response contains %q and should not", w)
		}
	}
}

// chordCards lists the chords the sheet's appendix names, in order. It reads the
// appendix rather than the lyric lines because the appendix holds each chord once
// and holds nothing else, which makes it the cheapest honest answer to "what
// chords is this sheet in".
func chordCards(body string) []string {
	var out []string
	for _, m := range chordCardPattern.FindAllStringSubmatch(body, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

var chordCardPattern = regexp.MustCompile(`(?s)<article class="chordcard.*?<h3>(.*?)</h3>`)

// --- the collection the tests read from ---

const amazingGrace = `---
title: Amazing Grace
artist: Traditional
key: G
genre: hymn
---

## Verse 1
A[G]mazing grace how [G7]sweet the [C]sound
That [G]saved a wretch like [D]me
`

const jazzTest = `---
title: Jazz
artist: Prueba
key: C
genre: jazz
---

## Estrofa 1
[Cmaj7#11]uno [Am9]dos [D13]tres [Gmaj9]cuatro
`

const withCapo = `---
title: Con cejilla
artist: Otro
key: G
capo: 2
genre: folk
---

## Estrofa 1
[G]uno [C]dos [D]tres
`

// seed fills a fresh instance with a small collection covering the shapes the
// pages have to handle: a plain song, one with a capo, and one whose chords have
// no known shape.
func (in *instance) seed() {
	in.t.Helper()
	in.writeSong("Traditional/Amazing Grace.song", amazingGrace)
	in.writeSong("Prueba/Jazz.song", jazzTest)
	in.writeSong("Otro/Con cejilla.song", withCapo)
}
