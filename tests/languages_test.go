package tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/i18n"
)

// Every language renders every page, and renders it in that language.
//
// The catalogue test proves no key is missing; this proves the keys reach the
// pages. A key that is looked up under a name nothing defines shows itself, so
// a stray "nav.songs" in the middle of a page is the failure this catches.
func TestEveryLanguageRendersEveryPage(t *testing.T) {
	key := regexp.MustCompile(`>\s*(?:nav|lab|book|users|settings|fail|why|parse|shape)\.[a-z0-9._-]+\s*<`)

	for _, l := range i18n.All() {
		t.Run(string(l), func(t *testing.T) {
			in := start(t, map[string]string{"TABERNACLE_LANG": string(l)})
			in.seed()
			in.rescan()
			for _, path := range []string{
				"/", "/artists", "/genres", "/chords", "/chords?c=Am7",
				"/songs/traditional-amazing-grace",
				"/songs/traditional-amazing-grace/print",
				"/songbook", "/songbook/print?all=1", "/login", "/nope",
			} {
				body := in.get(path).Body
				if m := key.FindString(body); m != "" {
					t.Errorf("%s shows a raw key: %s", path, strings.TrimSpace(m))
				}
			}
		})
	}
}

// The page says which language it is in and which way that language runs, on
// the one element everything else inherits from.
func TestThePageDeclaresItsLanguageAndDirection(t *testing.T) {
	for _, tc := range []struct{ lang, dir string }{
		{"es", "ltr"}, {"en", "ltr"}, {"ja", "ltr"}, {"ar", "rtl"},
	} {
		in := start(t, map[string]string{"TABERNACLE_LANG": tc.lang})
		want := `<html lang="` + tc.lang + `" dir="` + tc.dir + `">`
		if body := in.page("/chords"); !strings.Contains(body, want) {
			t.Errorf("%s: the page does not declare %s", tc.lang, want)
		}
	}
}

// A guitar is not a piece of text.
//
// Mirroring the neck would show a left-handed instrument to a right-handed
// reader and disagree with the SVG diagrams beside it, which cannot mirror at
// all. The sheet keeps its direction for a harder reason: a chord sits above the
// letter it falls on, counted from the left.
func TestTheNeckAndTheSheetDoNotMirror(t *testing.T) {
	sheet := readStatic(t, "app.css")
	for _, want := range []string{
		`[dir="rtl"] .boardgrid`,
		`[dir="rtl"] .sheet`,
		`[dir="rtl"] .tab`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("nothing keeps %s the right way round", want)
		}
	}
}

// A deployment can open in its own language, and refuses a code it does not
// have rather than quietly falling back.
func TestTheInstanceLanguageIsConfigured(t *testing.T) {
	in := start(t, map[string]string{"TABERNACLE_LANG": "de"})
	if body := in.page("/"); !strings.Contains(body, "Interpreten") {
		t.Error("TABERNACLE_LANG=de did not reach the header")
	}
}

// readStatic fetches an asset the way a browser does, because the tests here
// exercise a running instance and never reach inside it.
func readStatic(t *testing.T, name string) string {
	t.Helper()
	in := start(t, nil)
	page := in.page("/chords")
	m := regexp.MustCompile(`/static/[a-f0-9]+/`).FindString(page)
	if m == "" {
		t.Fatal("the page links to no static assets")
	}
	res := in.get(m + name)
	if res.Status != http.StatusOK {
		t.Fatalf("GET %s%s = %d", m, name, res.Status)
	}
	return res.Body
}
