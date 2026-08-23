package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/account"
)

// selectorLiteral finds the CSS selectors the script looks things up by, in
// either kind of quote.
var selectorLiteral = regexp.MustCompile(
	`(?:querySelector|querySelectorAll|closest|matches)\(\s*["']([^"']+)["']`)

// A selector in the script has to match something the page renders.
//
// This is the test that was missing twice. Splitting the neck into one radio
// group per string left the script listening for input[name="s"], and renaming
// the region it swaps left it looking for #chordresult: both silently did
// nothing, and every other test passed, because the markup was right and the Go
// was right. What breaks in a progressive enhancement is the join between them.
//
// It reads the selectors out of app.js rather than repeating them, so a renamed
// class cannot be fixed in one place and forgotten in the other.
func TestTheScriptLooksForThingsThatExist(t *testing.T) {
	script := asset(t, "static/app.js")

	// Enough of the application that every state the script reaches into is
	// rendered somewhere: the chord page, a song, both print views, the login
	// form, and a page carrying a notice.
	ts := newTestServer(t, basicSongs())
	pages := []string{}
	for _, path := range []string{
		"/chords?c=F", "/chords?c=Am7&all=1", "/chords/fragment?c=C&start=5",
		"/songs/traditional-amazing-grace", "/songs/traditional-amazing-grace/print",
		"/songbook", "/songbook/print?all=1", "/login", "/",
	} {
		pages = append(pages, ts.get(t, path).Body.String())
	}
	pages = append(pages, flashPage(t, ts))
	have := markupOf(pages)

	seen := map[string]bool{}
	for _, m := range selectorLiteral.FindAllStringSubmatch(script, -1) {
		sel := m[1]
		// A selector built by joining strings is captured only as far as its
		// first quote, which is not a selector. Unbalanced brackets are how that
		// shows.
		if seen[sel] || strings.Count(sel, "[") != strings.Count(sel, "]") {
			continue
		}
		seen[sel] = true
		for _, token := range tokensOf(sel) {
			if !have[token] {
				t.Errorf("the script looks for %q, and %s is on no page", sel, token)
			}
		}
	}
	if len(seen) < 8 {
		t.Fatalf("only %d selectors came out of the script; the regex has stopped working", len(seen))
	}
}

var (
	idAttr    = regexp.MustCompile(`\bid="([^"]+)"`)
	classAttr = regexp.MustCompile(`\bclass="([^"]*)"`)
	pairAttr  = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9-]*)="([^"]*)"`)
	bareAttr  = regexp.MustCompile(`(data-[a-zA-Z0-9-]+)`)
)

// markupOf collects what the pages contain, as exact names rather than as text
// to search through. A substring test would let .boardcel match boardcell, which
// is the kind of slip this exists to catch.
//
// Attribute values are stored unquoted, because a selector may write them either
// way and both mean the same thing.
func markupOf(pages []string) map[string]bool {
	have := map[string]bool{}
	for _, body := range pages {
		for _, m := range idAttr.FindAllStringSubmatch(body, -1) {
			have["#"+m[1]] = true
		}
		for _, m := range classAttr.FindAllStringSubmatch(body, -1) {
			for _, c := range strings.Fields(m[1]) {
				have["."+c] = true
			}
		}
		for _, m := range pairAttr.FindAllStringSubmatch(body, -1) {
			have["["+m[1]+"]"] = true
			have["["+m[1]+"="+m[2]+"]"] = true
		}
		// What is left once every name="value" is out of the way is the
		// attributes written without one.
		for _, m := range bareAttr.FindAllStringSubmatch(pairAttr.ReplaceAllString(body, " "), -1) {
			have["["+m[1]+"]"] = true
		}
	}
	return have
}

// tokensOf reduces a selector to the pieces that must exist in the markup: an
// id, a class, an attribute, an attribute with a value. Tag names, combinators
// and pseudo-classes are the browser's business.
func tokensOf(sel string) []string {
	sel = strings.NewReplacer(":checked", "", ":hover", "", ":focus-visible", "").Replace(sel)
	var out []string
	for part := sel; ; {
		i := strings.IndexAny(part, "#.[")
		if i < 0 {
			return out
		}
		open, rest := part[i], part[i+1:]
		end := strings.IndexAny(rest, " #.[")
		if open == '[' {
			end = strings.Index(rest, "]")
		}
		if end < 0 {
			end = len(rest)
		}
		switch name := rest[:end]; open {
		case '#':
			out = append(out, "#"+name)
		case '.':
			out = append(out, "."+name)
		default:
			out = append(out, "["+strings.ReplaceAll(name, `"`, "")+"]")
		}
		part = strings.TrimPrefix(rest[end:], "]")
	}
}

// classAdd finds the classes the script puts on things.
var classAdd = regexp.MustCompile(`classList\.add\(\s*["']([^"']+)["']`)

// A class the script adds has to mean something to the stylesheet.
//
// The other half of the same join: a selector that matches nothing is one way to
// break a progressive enhancement, and a class nothing styles is the other. The
// script hides the controls it has made redundant by adding one, and if the rule
// it relies on is renamed the buttons simply stay on the page.
func TestTheClassesTheScriptAddsAreStyled(t *testing.T) {
	script, sheet := asset(t, "static/app.js"), asset(t, "static/app.css")

	found := 0
	for _, m := range classAdd.FindAllStringSubmatch(script, -1) {
		found++
		if !strings.Contains(sheet, "."+m[1]) {
			t.Errorf("the script adds the class %q and no rule mentions it", m[1])
		}
	}
	if found == 0 {
		t.Fatal("no added classes came out of the script; the regex has stopped working")
	}
}

func asset(t *testing.T, name string) string {
	t.Helper()
	b, err := staticFS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// flashPage is the library with a notice on it, which is the only state the
// dismiss button is rendered in.
func flashPage(t *testing.T, ts *testServer) string {
	t.Helper()
	ts.as(t, "jose", account.RoleAdmin)
	w := ts.post(t, "/logout", url.Values{})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /logout = %d, want a redirect that sets a notice", w.Code)
	}
	ts.carry(w)
	return ts.get(t, "/").Body.String()
}
