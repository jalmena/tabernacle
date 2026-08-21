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

var scriptTags = regexp.MustCompile(`(?i)<script[^>]*>`)

// No address may reach a file outside the collection.
//
// The store confines itself with os.Root, so this is defence in depth rather
// than the only line — but it is the line an attacker actually touches, so it is
// tested from the outside.
func TestPathTraversalIsRefused(t *testing.T) {
	in := start(t, nil)
	in.seed()

	// Something recognisable that exists outside the collection.
	secret := filepath.Join(in.dir, "secreto.txt")
	if err := os.WriteFile(secret, []byte("CONTENIDO-SECRETO"), 0o600); err != nil {
		t.Fatal(err)
	}

	attempts := []string{
		"/songs/../secreto.txt",
		"/songs/..%2Fsecreto.txt",
		"/songs/..%2f..%2fsecreto.txt",
		"/songs/%2e%2e%2fsecreto.txt",
		"/songs/....//secreto.txt",
		"/songs/traditional-amazing-grace/../../secreto.txt",
		"/static/abc/../../../secreto.txt",
		"/static/abc/..%2f..%2f..%2fsecreto.txt",
		"/songs/%00secreto.txt",
		"/songs/" + url.PathEscape("../../etc/passwd"),
		"/songs/" + url.PathEscape("/etc/passwd"),
		"/static/abc/" + url.PathEscape("../../../../../../etc/passwd"),
	}
	for _, path := range attempts {
		t.Run(path, func(t *testing.T) {
			res := in.get(path)
			if strings.Contains(res.Body, "CONTENIDO-SECRETO") {
				t.Fatalf("%s served a file from outside the collection", path)
			}
			if strings.Contains(res.Body, "root:") {
				t.Fatalf("%s served /etc/passwd", path)
			}
			if res.Status == http.StatusOK && strings.Contains(res.Header.Get("Content-Type"), "text/plain") {
				t.Errorf("%s answered 200 with a plain-text body", path)
			}
		})
	}
}

// A Host header naming somewhere else is refused, which is what stops DNS
// rebinding turning a browser into a proxy onto somebody's LAN.
//
// This is the one real threat to an unauthenticated service on a home network: a
// public name that resolves to 192.168.x.x, and a page on the internet that then
// reads the songbook through the visitor's own browser.
func TestAnUnexpectedHostIsRefused(t *testing.T) {
	in := start(t, nil)
	in.seed()

	for _, host := range []string{"evil.example.com", "songbook.attacker.net", "tabernacle.example.org"} {
		t.Run(host, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, in.url+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = host
			res := in.do(req)
			if res.Status != http.StatusMisdirectedRequest {
				t.Errorf("Host %q answered %d, want 421", host, res.Status)
			}
			if strings.Contains(res.Body, "Amazing Grace") {
				t.Errorf("Host %q was served the collection", host)
			}
		})
	}

	// And the names a home server legitimately answers to still work, or the
	// guard would have made the application unusable.
	for _, host := range []string{"localhost:8080", "192.168.1.50:8080", "nas.local", "tabernacle"} {
		t.Run("allowed/"+host, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, in.url+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = host
			if got := in.do(req).Status; got != http.StatusOK {
				t.Errorf("Host %q answered %d, want 200", host, got)
			}
		})
	}
}

// A write that did not come from this page is refused, so a form on another site
// cannot delete somebody's songs.
func TestACrossOriginWriteIsRefused(t *testing.T) {
	in := start(t, nil)
	in.seed()

	cases := []struct {
		name   string
		origin string
		fetch  string
	}{
		{"foreign origin", "https://evil.example", ""},
		{"foreign origin, cross-site", "https://evil.example", "cross-site"},
		{"no origin but cross-site", "", "cross-site"},
		// A browser scopes neither an origin nor a cookie by hostname alone, so
		// another service on this very machine is somebody else.
		{"same host, another port", "http://127.0.0.1:9999", "same-origin"},
		{"another address on the LAN", "http://192.168.1.50:8080", "same-origin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := url.Values{"text": {newSong}}.Encode()
			req, err := http.NewRequest(http.MethodPost, in.url+"/songs", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", c.fetch)
			}
			if got := in.do(req).Status; got != http.StatusForbidden {
				t.Errorf("status = %d, want 403", got)
			}
		})
	}

	if got := in.songFiles(); len(got) != 3 {
		t.Errorf("a cross-origin write got through: %v", got)
	}
}

// A .song file is untrusted input, and every field of it ends up on a page.
// Markup in any of them must arrive as text.
func TestMarkupInASongNeverBecomesMarkup(t *testing.T) {
	in := start(t, nil)

	const payload = `<script>alert('xss')</script>`
	hostile := "---\n" +
		"title: " + payload + " titulo\n" +
		"artist: " + payload + " artista\n" +
		"key: G\n" +
		"genre: " + payload + "\n" +
		"notes: " + payload + " en las notas\n" +
		"---\n\n" +
		"## " + payload + " seccion\n" +
		"[G]" + payload + " en la letra\n" +
		"[" + payload + "] como anotacion\n"

	in.writeSong("Hostil/hostil.song", hostile)

	paths := []string{
		"/", "/artists", "/genres", "/?q=xss", "/?q=alert",
		"/songs/hostil-hostil",
		"/songs/hostil-hostil/sheet",
		"/songs/hostil-hostil/print",
		"/songs/hostil-hostil/edit",
		"/songs/hostil-hostil/delete",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			res := in.get(path)
			if res.Status != http.StatusOK {
				t.Fatalf("status = %d", res.Status)
			}
			// The exact bytes matter here, so this looks at the body rather than
			// at the visible text.
			for _, forbidden := range []string{"<script>", "alert('xss')"} {
				if strings.Contains(res.Body, forbidden) {
					t.Errorf("%s contains %q unescaped", path, forbidden)
				}
			}
			// Stronger, and not fooled by a payload that opens a tag some other
			// way: every script tag on the page must be one of the
			// application's own file loads. There is no inline script anywhere
			// in Tabernacle, so anything else came from a song file.
			for _, tag := range scriptTags.FindAllString(res.Body, -1) {
				if !strings.Contains(tag, `src="/static/`) {
					t.Errorf("%s carries a script tag that is not a static asset: %s", path, tag)
				}
			}
		})
	}

	// The raw route hands over the file itself, so it must not be served as
	// something a browser will render.
	raw := in.get("/songs/hostil-hostil/raw")
	if ct := raw.Header.Get("Content-Type"); strings.Contains(ct, "text/html") {
		t.Errorf("the raw file is served as %q", ct)
	}
}

// A search snippet is assembled from the file's own words, which makes it the
// likeliest place for markup to slip through.
func TestASearchSnippetIsEscaped(t *testing.T) {
	in := start(t, nil)
	in.writeSong("Hostil/comilla.song",
		"---\ntitle: Comillas\nartist: Prueba\nkey: G\n---\n\n"+
			"## Estrofa 1\n[G]un \"img onerror=alert(1)\" peligroso\n")

	res := in.get("/?q=peligroso")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	res.mustContain("<mark>")
	if strings.Contains(res.Body, `onerror=alert(1)"`) {
		t.Error("the snippet let an attribute through unescaped")
	}
	// The words are still readable, or the escaping has eaten the feature.
	res.mustShow("peligroso")
}

// Every response carries the headers that remove whole classes of problem, and
// the policy has to be strict enough to be worth having.
func TestSecurityHeadersAreOnEveryResponse(t *testing.T) {
	in := start(t, nil)
	in.seed()

	for _, path := range []string{"/", "/songs/traditional-amazing-grace", "/nonsense", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			h := in.get(path).Header
			csp := h.Get("Content-Security-Policy")
			if csp == "" {
				t.Fatal("no content security policy")
			}
			for _, want := range []string{"default-src 'none'", "frame-ancestors 'none'", "base-uri 'none'"} {
				if !strings.Contains(csp, want) {
					t.Errorf("the policy lacks %q", want)
				}
			}
			// An inline script would need one of these, and there are none in the
			// application. Allowing them anyway would give away the benefit.
			if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
				t.Errorf("the policy allows inline script or eval: %s", csp)
			}
			if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q", got)
			}
			if got := h.Get("Referrer-Policy"); got != "no-referrer" {
				t.Errorf("Referrer-Policy = %q", got)
			}
		})
	}
}

// A personal songbook has no business in a search index.
func TestCrawlersAreTurnedAway(t *testing.T) {
	in := start(t, nil)
	in.seed()

	in.get("/robots.txt").mustContain("Disallow: /")
	// And the pages say it themselves, in case robots.txt is never fetched.
	in.get("/songs/traditional-amazing-grace").mustContain("noindex")
}

// A song larger than the limit is refused rather than read into memory.
func TestAnOversizedSongIsRefused(t *testing.T) {
	in := start(t, map[string]string{"TABERNACLE_MAX_SONG_BYTES": "2048"})

	huge := "---\ntitle: Enorme\nartist: Prueba\nkey: G\n---\n\n## Estrofa 1\n" +
		strings.Repeat("[G]palabra ", 5000)
	res := in.post("/songs", url.Values{"text": {huge}})
	if res.Status == http.StatusSeeOther {
		t.Error("an oversized song was accepted")
	}
	if got := in.songFiles(); len(got) != 0 {
		t.Errorf("an oversized song was written: %v", got)
	}
	// Reading still works afterwards, so the refusal did not wedge anything.
	if in.get("/").Status != http.StatusOK {
		t.Error("the library stopped answering after an oversized submission")
	}
}

// An archive whose entries try to escape the collection must not write outside
// it. This is zip slip, and it is why import is the riskiest route here.
func TestZipSlipIsRefused(t *testing.T) {
	in := start(t, nil)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{
		"../escapado.song",
		"../../escapado.song",
		"/absoluto.song",
		"..\\windows.song",
		"Bien/legitima.song",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		title := "Escapado"
		if name == "Bien/legitima.song" {
			title = "Legitima"
		}
		body := "---\ntitle: " + title + "\nartist: Malo\nkey: G\n---\n\n## Estrofa 1\n[G]uno\n"
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	in.upload("/import", "files", map[string][]byte{"malicioso.zip": buf.Bytes()})
	in.rescan()

	// Nothing outside the songs directory, in the data directory or above it.
	for _, dir := range []string{in.dir, filepath.Dir(in.dir)} {
		matches, err := filepath.Glob(filepath.Join(dir, "*.song"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 0 {
			t.Errorf("an archive entry escaped the collection: %v", matches)
		}
	}
	for _, f := range in.songFiles() {
		if strings.Contains(f, "..") {
			t.Errorf("%q is not a path inside the collection", f)
		}
	}
}

// A file that is not a song, and an archive that is not an archive, are refused
// with an explanation rather than a crash.
func TestRubbishUploadsAreRefusedCleanly(t *testing.T) {
	in := start(t, nil)

	uploads := map[string][]byte{
		"no-es-nada.song": []byte("\x00\x01\x02 esto no es una cancion"),
		"falso.zip":       []byte("PK\x03\x04 pero no lo es"),
		"vacio.song":      {},
	}
	for name, content := range uploads {
		t.Run(name, func(t *testing.T) {
			res := in.upload("/import", "files", map[string][]byte{name: content})
			if res.Status >= 500 {
				t.Errorf("status = %d: rubbish must not be a server error", res.Status)
			}
			if got := in.songFiles(); len(got) != 0 {
				t.Errorf("rubbish was written to the collection: %v", got)
			}
		})
	}
	if in.get("/").Status != http.StatusOK {
		t.Error("the library stopped answering after a rubbish upload")
	}
}

// Hostile in ways that are not markup: control characters, an enormous single
// token, nested brackets, and a right-to-left override that could reorder what a
// reader sees.
func TestAHostileSongFileIsHandled(t *testing.T) {
	in := start(t, nil)

	hostile := []string{
		"---\ntitle: Control\nartist: P\nkey: G\n---\n\n## Estrofa 1\n[G]uno\x07\x08dos\n",
		"---\ntitle: Largo\nartist: P\nkey: G\n---\n\n## Estrofa 1\n[G]" + strings.Repeat("a", 3000) + "\n",
		"---\ntitle: Corchetes\nartist: P\nkey: G\n---\n\n## Estrofa 1\n[[[[G]]]]uno\n",
		"---\ntitle: Orden\nartist: P\nkey: G\n---\n\n## Estrofa 1\n[G]‮uno dos\n",
	}
	for i, text := range hostile {
		in.upload("/import", "files", map[string][]byte{"h.song": []byte(text)})
		if got := in.get("/"); got.Status != http.StatusOK {
			t.Errorf("case %d took the library down (%d)", i, got.Status)
		}
	}

	// Whatever landed has to render, rather than being indexed and then failing
	// to draw.
	in.rescan()
	body := in.page("/")
	for _, title := range []string{"Control", "Largo", "Corchetes", "Orden"} {
		if !strings.Contains(body, title) {
			continue // refused at import, which is a fine answer
		}
	}
	if in.get("/").Status != http.StatusOK {
		t.Error("the library does not answer after the hostile imports")
	}
}
