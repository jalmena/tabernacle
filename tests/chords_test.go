package tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The chord page is a reference, and a reference answers every reasonable
// question about it without a script and without an account.
func TestTheChordPageIsReadableByAnyone(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	anon := in.anon()

	body := anon.page("/chords?c=Am7")
	for _, want := range []string{"Acordes", "Am7", "Primera posición", "<svg"} {
		if !strings.Contains(body, want) {
			t.Errorf("the chord page is missing %q", want)
		}
	}
}

// It works with JavaScript off, because the neck is a form and not a drawing.
//
// The check is deliberately about the markup rather than about running the
// script: what makes the page usable without one is that the state is in the
// query, the controls are native inputs, and the form submits by itself.
func TestTheChordPageWorksWithoutScripting(t *testing.T) {
	in := start(t, nil)
	body := in.page("/chords")

	if !strings.Contains(body, `<form class="board" id="fretboard" method="get" action="/chords"`) {
		t.Error("the neck is not a plain GET form")
	}
	if strings.Count(body, "<fieldset") < 6 {
		t.Error("the neck does not give each string its own group of choices")
	}
	// One radio group per string, or setting the second would clear the first.
	for i := range 6 {
		if !strings.Contains(body, `name="s`+string(rune('0'+i))+`"`) {
			t.Errorf("string %d has no radio group of its own", i)
		}
	}
	// The script is only ever a src, never an inline block that a policy would
	// have to allow.
	if regexp.MustCompile(`(?s)<script[^>]*>\s*[^\s<]`).MatchString(body) {
		t.Error("the page carries inline script")
	}

	// And submitting that form by hand identifies the shape.
	got := in.page("/chords?s0=x&s1=3&s2=2&s3=0&s4=1&s5=0")
	if !strings.Contains(got, `<span class="rname">C</span>`) {
		t.Error("the submitted form did not identify the open C shape")
	}
}

// The fragment the script fetches and the region the page renders are the same
// markup, so enhancing the page cannot change what it says.
func TestTheFragmentAndThePageAgree(t *testing.T) {
	in := start(t, nil)
	const query = "?c=Am7&s0=x&s1=0&s2=2&s3=0&s4=1&s5=0"

	inPage := resultRegion(t, in.page("/chords"+query))
	fragment := strings.TrimSpace(in.page("/chords/fragment" + query))

	if inPage != fragment {
		t.Errorf("the fragment and the page differ\npage:\n%s\n\nfragment:\n%s",
			trim(inPage), trim(fragment))
	}
}

var resultBlock = regexp.MustCompile(`(?s)<div id="chordresult">(.*)</div>\s*</main>`)

func resultRegion(t *testing.T, body string) string {
	t.Helper()
	m := resultBlock.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the page has no #chordresult region")
	}
	return strings.TrimSpace(m[1])
}

func trim(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// Nothing a reader can put in the query produces an error page. A reference that
// answers a fumbled URL with a 400 is telling somebody they typed wrong when it
// could have shown them the chords.
func TestTheChordPageNeverRefusesInput(t *testing.T) {
	in := start(t, nil)
	for _, q := range []string{
		"", "?c=", "?c=Zx9", "?c=<script>alert(1)</script>", "?c=" + strings.Repeat("A", 500),
		"?grip=hola", "?grip=x-x-x", "?grip=1-2-3-4-5-6-7-8-9", "?grip=x-3-2-0-1-99",
		"?start=0", "?start=-5", "?start=999", "?start=abc",
		"?s0=x", "?s0=99&s1=x&s2=x&s3=x&s4=x&s5=x", "?all=1", "?all=perhaps",
		"?c=C&grip=x-3-2-0-1-0&start=7&all=1",
	} {
		if got := in.get("/chords" + q).Status; got != http.StatusOK {
			t.Errorf("GET /chords%s = %d, want 200", q, got)
		}
		if got := in.get("/chords/fragment" + q).Status; got != http.StatusOK {
			t.Errorf("GET /chords/fragment%s = %d, want 200", q, got)
		}
	}
}

// A symbol that is not a chord is echoed back inside the page, so it has to be
// escaped there.
func TestTheChordPageEscapesWhatWasTyped(t *testing.T) {
	in := start(t, nil)
	body := in.page("/chords?c=" + "%3Cimg+src%3Dx+onerror%3Dalert(1)%3E")
	if strings.Contains(body, "<img src=x") {
		t.Error("the typed symbol reached the page as markup")
	}
	if !strings.Contains(body, "&lt;img") {
		t.Error("the typed symbol was not echoed back at all")
	}
}

// Reading is reading: the chord page changes nothing, however often it is asked.
func TestTheChordPageChangesNothing(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.rescan()
	before := in.songFiles()

	for _, q := range []string{"?c=C", "?c=Am7&all=1", "?grip=x-3-2-0-1-0", "?start=5"} {
		in.page("/chords" + q)
		in.page("/chords" + q)
	}
	if got := in.songFiles(); strings.Join(got, "|") != strings.Join(before, "|") {
		t.Errorf("the chord page touched the collection: %v became %v", before, got)
	}
}

// The chord reference is reachable from everywhere, which is the point of it
// being a section rather than a page somebody has to know about.
func TestTheChordSectionIsInTheHeader(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.rescan()
	for _, path := range []string{"/", "/artists", "/genres", "/chords"} {
		if !strings.Contains(in.page(path), `href="/chords"`) {
			t.Errorf("%s does not offer the chord reference", path)
		}
	}
}

// The shapes on the page are the ones a player is taught, and the first one is
// the one they are taught first.
func TestTheChordPageLeadsWithTheShapeEveryoneKnows(t *testing.T) {
	in := start(t, nil)
	first := regexp.MustCompile(`class="labshape[^"]*" href="[^"]*grip=([x0-9-]+)`)
	for sym, want := range map[string]string{
		"C":  "x-3-2-0-1-0",
		"F":  "1-3-3-2-1-1",
		"G":  "3-2-0-0-3-3",
		"Am": "x-0-2-2-1-0",
		"D":  "x-x-0-2-3-2",
		"E":  "0-2-2-1-0-0",
	} {
		m := first.FindStringSubmatch(in.page("/chords?c=" + sym))
		if m == nil {
			t.Errorf("%s shows no shapes at all", sym)
			continue
		}
		if m[1] != want {
			t.Errorf("%s leads with %s, want %s", sym, m[1], want)
		}
	}
}

// An inversion is a different chord and gets its own page, rather than being
// piled onto this one under a caption that would be a lie.
func TestInversionsAreOfferedSeparately(t *testing.T) {
	in := start(t, nil)
	body := in.page("/chords?c=C")
	if !strings.Contains(body, "Con otra nota en el bajo") {
		t.Fatal("the page does not offer the inversions")
	}
	if !strings.Contains(body, "/chords?c=C%2FE") {
		t.Error("C/E is not among them")
	}
	// And that page really is about C/E: every shape has E underneath.
	over := in.page("/chords?c=C/E")
	if !strings.Contains(over, "<h2>C/E") {
		t.Errorf("the C/E page is not headed C/E")
	}
}
