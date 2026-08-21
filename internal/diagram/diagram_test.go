package diagram

import (
	"encoding/xml"
	"strings"
	"testing"
)

// C major, open position.
var cMajor = Input{
	Strings: 6,
	Frets:   []int{Muted, 3, 2, 0, 1, 0},
	Fingers: []int{0, 3, 2, 0, 1, 0},
	Label:   "C",
}

// F barre at the first fret.
var fBarre = Input{
	Strings: 6,
	Frets:   []int{1, 3, 3, 2, 1, 1},
	Fingers: []int{1, 4, 3, 2, 1, 1},
	Barres:  []Barre{{Fret: 1, From: 0, To: 5, Finger: 1}},
	Label:   "F",
}

// A minor seventh high on the neck, so the window has to start above the nut.
var am7High = Input{
	Strings:   6,
	Frets:     []int{Muted, 12, 14, 12, 13, 12},
	Fingers:   []int{0, 1, 3, 1, 2, 1},
	Barres:    []Barre{{Fret: 12, From: 1, To: 5, Finger: 1}},
	StartFret: 12,
	Label:     "Am7",
}

func TestOutputIsWellFormedXML(t *testing.T) {
	for name, in := range map[string]Input{
		"C major":     cMajor,
		"F barre":     fBarre,
		"Am7 high":    am7High,
		"all muted":   {Strings: 6, Frets: []int{Muted, Muted, Muted, Muted, Muted, Muted}, Label: "?"},
		"all open":    {Strings: 6, Frets: []int{0, 0, 0, 0, 0, 0}, Label: "Em7"},
		"four string": {Strings: 4, Frets: []int{Muted, Muted, 2, 3}, Label: "D"},
		"zero value":  {},
		"thumb":       {Strings: 6, Frets: []int{2, 0, 0, 2, 3, 2}, Fingers: []int{5, 0, 0, 1, 3, 2}, Label: "D/F#"},
	} {
		t.Run(name, func(t *testing.T) {
			out := String(in)
			// Decoding proves the escaping is right; a golden diff of broken XML
			// is unreadable.
			if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
				t.Fatalf("output is not well-formed XML: %v\n%s", err, out)
			}
			if !strings.HasPrefix(out, "<svg ") || !strings.HasSuffix(out, "</svg>") {
				t.Errorf("output is not a single svg element:\n%s", out)
			}
		})
	}
}

// The whole reason this package exists in isolation: it must be usable in a
// light theme, a dark theme and on paper without being re-rendered.
func TestNoColoursAreEmitted(t *testing.T) {
	for _, in := range []Input{cMajor, fBarre, am7High} {
		out := String(in)
		for _, forbidden := range []string{"#", "rgb(", "hsl(", "black", "white", "fill=\"none\" stroke=\"#"} {
			if forbidden == "#" {
				// A hash only appears in a colour here, since the label is
				// escaped and no id is referenced.
				if strings.Contains(out, `="#`) {
					t.Errorf("output contains a hex colour:\n%s", out)
				}
				continue
			}
			if strings.Contains(out, forbidden) {
				t.Errorf("output contains %q:\n%s", forbidden, out)
			}
		}
		if !strings.Contains(out, "currentColor") {
			t.Error("output does not use currentColor, so it cannot follow the theme")
		}
	}
}

func TestInlineOmitsTheNamespace(t *testing.T) {
	var inline strings.Builder
	if err := Inline(&inline, cMajor); err != nil {
		t.Fatal(err)
	}
	// Inside HTML the SVG namespace is implied, so declaring it is noise repeated
	// once per diagram.
	if strings.Contains(inline.String(), "xmlns") {
		t.Error("the output carries an xmlns attribute")
	}
}

func TestStructure(t *testing.T) {
	out := String(cMajor)

	// One dot per fretted string: frets 3, 2 and 1.
	if got := strings.Count(out, "<circle"); got != 3+2 {
		// Three dots plus two open-string rings.
		t.Errorf("got %d circles, want 3 dots and 2 open rings", got)
	}
	// One cross for the muted low E.
	if got := strings.Count(out, `stroke-width="1.3"`); got != 1 {
		t.Errorf("got %d muted markers, want 1", got)
	}
	// Finger numbers for the three fretted strings.
	if got := strings.Count(out, "<text"); got != 3 {
		t.Errorf("got %d finger labels, want 3", got)
	}
	// An open-position shape draws a nut and no fret number.
	if !strings.Contains(out, `stroke-width="3"`) {
		t.Error("an open-position diagram has no nut")
	}

	high := String(am7High)
	if strings.Contains(high, `stroke-width="3"`) {
		t.Error("a shape at the twelfth fret should not draw a nut")
	}
	if !strings.Contains(high, ">12</text>") {
		t.Error("a shape above the nut must say which fret it starts at")
	}
	if !strings.Contains(high, "<rect") {
		t.Error("the barre is missing")
	}
}

func TestBarreIsDrawnUnderTheDots(t *testing.T) {
	out := String(fBarre)
	rect := strings.Index(out, "<rect")
	dot := strings.Index(out, "<circle")
	if rect < 0 {
		t.Fatal("no barre was drawn")
	}
	if dot >= 0 && rect > dot {
		t.Error("the barre is drawn after the dots, so it would cover them")
	}
}

// A shape that does not fit the default window must widen it rather than draw
// notes outside the picture.
func TestWindowGrowsToFitTheShape(t *testing.T) {
	wide := Input{
		Strings: 6,
		Frets:   []int{Muted, 1, 3, 5, 7, Muted}, // a six-fret span
		Label:   "stretch",
	}
	out := String(wide)
	if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
		t.Fatalf("not well formed: %v", err)
	}
	// Every fretted note must have a dot, so none was silently dropped.
	if got := strings.Count(out, `stroke="none"/>`); got < 4 {
		t.Errorf("only %d dots drawn for four fretted notes:\n%s", got, out)
	}
}

// The label is the one place text from elsewhere enters, so it must be escaped
// here: templates treat this output as markup.
// The chord name itself is rendered as HTML text beside the diagram, not inside
// it, so that it stays selectable and styleable. The only text this package
// emits from elsewhere is the accessible title, and templates treat this output
// as markup, so it has to escape its own input.
func TestTitleTextIsEscaped(t *testing.T) {
	in := cMajor
	in.Title = `a "quoted" & <dangerous> title`
	out := String(in)

	if strings.Contains(out, "<dangerous>") {
		t.Error("the title was emitted as live markup")
	}
	for _, want := range []string{"&lt;dangerous&gt;", "&amp;", "&quot;"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing the escaped form %q:\n%s", want, out)
		}
	}
	if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
		t.Fatalf("escaping produced invalid XML: %v", err)
	}
}

// When no title is given the label is used as one, so it needs escaping too.
func TestLabelUsedAsTitleIsEscaped(t *testing.T) {
	in := cMajor
	in.Label = `<script>alert(1)</script>`
	in.Title = ""
	out := String(in)

	if strings.Contains(out, "<script>") {
		t.Error("the label was emitted as live markup")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("the label was not used as an escaped title:\n%s", out)
	}
	if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
		t.Fatalf("escaping produced invalid XML: %v", err)
	}
}

func TestTitleFallsBackToTheLabel(t *testing.T) {
	if !strings.Contains(String(cMajor), "<title>C</title>") {
		t.Error("with no explicit title, the label should be used")
	}
	in := cMajor
	in.Title = "Do mayor"
	if !strings.Contains(String(in), "<title>Do mayor</title>") {
		t.Error("an explicit title was not used")
	}
}

func TestFingersOnlyLabelFrettedStrings(t *testing.T) {
	in := Input{
		Strings: 6,
		Frets:   []int{Muted, 0, 2, 2, 2, 0},
		// Nonsense fingers on the muted and open strings must be ignored.
		Fingers: []int{3, 3, 1, 2, 3, 3},
		Label:   "A",
	}
	out := String(in)
	if got := strings.Count(out, "<text"); got != 3 {
		t.Errorf("got %d finger labels, want 3 (only the fretted strings)", got)
	}
}

func TestThumbIsLabelled(t *testing.T) {
	out := String(Input{
		Strings: 6,
		Frets:   []int{2, 0, 0, 2, 3, 2},
		Fingers: []int{5, 0, 0, 1, 3, 2},
		Label:   "D/F#",
	})
	if !strings.Contains(out, ">P<") {
		t.Error("a thumbed note is not labelled")
	}
}

// A malformed barre must not produce a diagram with a rectangle floating outside
// the grid.
func TestOutOfRangeBarreIsIgnored(t *testing.T) {
	for _, br := range []Barre{
		{Fret: 1, From: -1, To: 5},
		{Fret: 1, From: 0, To: 99},
		{Fret: 1, From: 4, To: 2},
		{Fret: 40, From: 0, To: 5},
	} {
		in := cMajor
		in.Barres = []Barre{br}
		out := String(in)
		if strings.Contains(out, "<rect") {
			t.Errorf("barre %+v was drawn despite being out of range", br)
		}
		if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
			t.Errorf("barre %+v produced invalid XML: %v", br, err)
		}
	}
}

// Short or absent fret data must not panic; a caller with bad data should get a
// diagram of muted strings, not a crash.
func TestShortInputIsPadded(t *testing.T) {
	for _, in := range []Input{
		{Strings: 6},
		{Strings: 6, Frets: []int{0}},
		{Strings: 6, Frets: []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{Strings: 1, Frets: []int{3}},
	} {
		out := String(in)
		if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
			t.Errorf("input %+v produced invalid XML: %v", in, err)
		}
	}
}

// The output goes straight into a page, so its size is a real cost.
func TestOutputStaysCompact(t *testing.T) {
	for name, in := range map[string]Input{"C": cMajor, "F": fBarre, "Am7": am7High} {
		if n := len(String(in)); n > 1400 {
			t.Errorf("%s renders to %d bytes, which is more than a diagram should cost", name, n)
		}
	}
}

func FuzzWrite(f *testing.F) {
	f.Add(6, -1, 3, 2, 0, 1, 0, 1, 5, "C")
	f.Add(6, 1, 3, 3, 2, 1, 1, 1, 5, "F")
	f.Fuzz(func(t *testing.T, strings_ int, f0, f1, f2, f3, f4, f5, start, window int, label string) {
		in := Input{
			Strings:   strings_,
			Frets:     []int{f0, f1, f2, f3, f4, f5},
			Fingers:   []int{1, 2, 3, 4, 1, 2},
			StartFret: start,
			Window:    window,
			Label:     label,
			Barres:    []Barre{{Fret: f0, From: 0, To: 5, Finger: 1}},
		}
		// Absurd geometry is a caller's mistake, but it must not panic or
		// produce something a browser cannot parse.
		if in.Strings > 64 || in.Strings < 0 || window > 64 || window < 0 {
			return
		}
		out := String(in)
		if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
			t.Fatalf("invalid XML for %+v: %v\n%s", in, err, out)
		}
	})
}

// XML cannot represent most control characters at all, not even as a numeric
// reference, so they have to be dropped rather than encoded. Found by the
// fuzzer, which is exactly what it is for.
func TestControlCharactersAreDropped(t *testing.T) {
	for _, title := range []string{
		"\x10", "C\x00major", "a\x1bb", "￾", "ok\x07",
		"C\x09tab", // a tab is legal and must survive
	} {
		in := cMajor
		in.Title = title
		out := String(in)
		if err := xml.Unmarshal([]byte(out), new(struct{})); err != nil {
			t.Errorf("title %q produced invalid XML: %v", title, err)
		}
	}
	// A title made only of illegal characters leaves no empty element behind.
	in := cMajor
	in.Title = "\x10\x11"
	in.Label = ""
	if strings.Contains(String(in), "<title>") {
		t.Error("an unusable title still emitted a title element")
	}
	// A legal character is not dropped along with the illegal ones.
	in.Title = "C\x10mayor"
	if !strings.Contains(String(in), "<title>Cmayor</title>") {
		t.Errorf("legal text was lost: %s", String(in))
	}
}
