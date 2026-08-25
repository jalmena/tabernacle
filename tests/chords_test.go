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
	for _, want := range []string{"Acordes", "Am7", "posiciones", "<svg"} {
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

	// The controls that only a reader without a script needs are there, and are
	// marked so the script can take them away once it has made them pointless.
	for _, want := range []string{
		`<button class="btn btn-primary fallback" type="submit">Identificar</button>`,
		`<button class="btn fallback" type="submit">Ver</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing its fallback control: %s", want)
		}
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

var resultBlock = regexp.MustCompile(`(?s)<div id="chordlab">(.*)</div>\s*</main>`)

func resultRegion(t *testing.T, body string) string {
	t.Helper()
	m := resultBlock.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the page has no #chordlab region")
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
	first := regexp.MustCompile(`class="labshape[^"]*"[^>]*data-grip="([x0-9-]+)"`)
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

// An empty page starts with the six strings ringing open, which is what a guitar
// does when nobody is holding it — not silent, which is what a row of crosses
// would say.
func TestTheChordPageStartsWithTheStringsOpen(t *testing.T) {
	in := start(t, nil)
	body := in.page("/chords")
	for i := range 6 {
		want := `name="s` + string(rune('0'+i)) + `" value="0" checked`
		if !strings.Contains(body, want) {
			t.Errorf("string %d does not start open", i)
		}
	}
}

// Naming a chord puts it on the neck. A reader who asks for F wants to see F,
// not an empty grid beside a list of pictures of it.
func TestNamingAChordDrawsIt(t *testing.T) {
	in := start(t, nil)
	for sym, want := range map[string][]string{
		"F":  {`name="s0" value="1" checked`, `name="s1" value="3" checked`},
		"Am": {`name="s0" value="x" checked`, `name="s2" value="2" checked`},
	} {
		body := in.page("/chords?c=" + sym)
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s did not reach the neck: missing %s", sym, w)
			}
		}
	}
	// And a shape asked for by name wins over the chord's own.
	drawn := in.page("/chords?c=F&grip=x-x-3-2-1-1")
	if !strings.Contains(drawn, `name="s0" value="x" checked`) {
		t.Error("an explicit shape did not reach the neck")
	}
}

// The shapes are one list, easiest first, and not a set of headings to read
// through. Somebody learning a chord wants the one they can play today and then
// the next one.
func TestTheShapesAreOneList(t *testing.T) {
	in := start(t, nil)
	body := in.page("/chords?c=Am7")

	if strings.Contains(body, "Primera posición") || strings.Contains(body, "<h3>Traste") {
		t.Error("the shapes are still divided into positions")
	}
	if n := strings.Count(body, `class="shaperow"`); n != 1 {
		t.Errorf("the shapes are in %d rows, want one list", n)
	}
	if n := strings.Count(body, "class=\"labshape"); n < 8 {
		t.Errorf("the list holds only %d shapes", n)
	}
	// The real total is printed, so a trimmed list does not read as all there
	// is. What it must not claim is that the order is by difficulty: a curated
	// shape leads, then the fuller and lower ones, and a three-string grip that
	// is the easiest thing on the page can sit near the end.
	if !strings.Contains(body, "posiciones, las más habituales primero") {
		t.Error("the list does not say how it is ordered")
	}
	if strings.Contains(body, "más fácil") {
		t.Error("the list still claims to be ordered by how easy the shapes are")
	}
}

// The reading the rest of the page is about is the one marked, and clicking
// another moves the mark rather than leaving it on the first.
func TestTheChosenReadingIsMarked(t *testing.T) {
	in := start(t, nil)
	const grip = "0-3-2-2-1-0" // Am with E underneath, which is also C6 without its fifth

	marked := regexp.MustCompile(`<a class="on" href="[^"]*" data-c="([^"]*)"`)

	first := marked.FindStringSubmatch(in.page("/chords?grip=" + grip))
	if first == nil {
		t.Fatal("nothing is marked when no chord has been chosen")
	}
	if first[1] != "Am/E" {
		t.Errorf("the likeliest reading is %s, want Am/E", first[1])
	}

	second := marked.FindStringSubmatch(in.page("/chords?c=C6%2FE&grip=" + grip))
	if second == nil {
		t.Fatal("nothing is marked after choosing a reading")
	}
	if second[1] != "C6/E" {
		t.Errorf("the chosen reading is %s, want C6/E", second[1])
	}
	// Exactly one, or the mark says nothing.
	if n := strings.Count(in.page("/chords?c=C6%2FE&grip="+grip), `<a class="on"`); n != 1 {
		t.Errorf("%d readings are marked at once", n)
	}
}

// Drawing a chord on an untouched neck leaves the thick strings ringing, which
// is the commonest mistake there is. The page names the chord anyway and says
// what to stop, and following it silences those strings.
func TestANearMissIsNamedAndCanBeFollowed(t *testing.T) {
	in := start(t, nil)

	body := in.page("/chords?grip=0-0-0-2-3-2") // D, with the sixth and fifth still open
	if !strings.Contains(body, "Casi") {
		t.Fatal("the page does not offer the near miss at all")
	}
	for _, want := range []string{
		`<span class="rname">D/A</span>`, "sin la 6ª",
		`<span class="rname">D</span>`, "sin la 6ª y la 5ª",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the near miss is missing %q", want)
		}
	}

	// Following the second one puts the real D on the neck and names it exactly.
	nearly := regexp.MustCompile(`data-c="D" data-grip="([x0-9-]+)"`).FindStringSubmatch(body)
	if nearly == nil {
		t.Fatal("the suggestion carries no shape to follow")
	}
	if nearly[1] != "x-x-0-2-3-2" {
		t.Fatalf("following D would draw %s, want x-x-0-2-3-2", nearly[1])
	}
	after := in.page("/chords?c=D&grip=" + nearly[1])
	if !strings.Contains(after, `<a class="on" href="/chords?c=D&amp;grip=x-x-0-2-3-2" data-c="D"`) {
		t.Error("after following it, D is not the reading the page is about")
	}
	if strings.Contains(after, "Casi") {
		t.Error("the near miss is still offered once the shape is exact")
	}
}

// An untouched neck is not handed a list of everything six open strings could
// almost be.
func TestAnUntouchedNeckSuggestsNothing(t *testing.T) {
	in := start(t, nil)
	if strings.Contains(in.page("/chords"), "Casi") {
		t.Error("the opening page offers near misses before anything is drawn")
	}
}
