package song

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Each chord lands in the words at the column it stood over, counting
// characters: the accent in "Canción" must not move the G.
func TestASheetPutsEachChordWhereItStood(t *testing.T) {
	for _, c := range []struct {
		name, sheet, want string
	}{
		{"over the words", "Am      G\nCanción inventada", "[Am]Canción [G]inventada"},
		{"past the end", "Dm          C\ncorto", "[Dm]corto [C]"},
		{"over the gap", "Am  G\nhola mundo", "[Am]hola [G]mundo"},
		{"indented words", "  F    E\n  para probar", "[F]para [E]probar"},
		{"solfege", "Lam  Fa    Mi7\notra línea más", "[Am]otra [F]línea [E7]más"},
		{"solfege in capitals", "LAm RE    MI7/SOL\nuna letra más", "[Am]una [D]letra [E7/G]más"},
		{"bars and repeats", "Am / / | G (x2)\nuna vuelta más aquí", "[Am]una vuelt[G]a [x2]más aquí"},
		{"tabs for spaces", "C\tG\nuna dos tres", "[C]una dos [G]tres"},
		{"no words under", "Am  G  F  E\n\nletra", "[Am] [G] [F] [E]\n\nletra"},
		{"repeat in words", "LAm RE SI (2 veces)", "[Am] [D] [B] [x2]"},
		{"trailing off", "LAm.....", "[Am]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := FromSheet(c.sheet, "Estrofa")
			if len(p.Sections) != 1 || p.Sections[0].Label != "" {
				t.Fatalf("sections = %#v", p.Sections)
			}
			if got := p.Sections[0].Body; got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

// Words that happen to start with something chord-shaped are still words.
func TestASheetLeavesWordsAlone(t *testing.T) {
	for _, line := range []string{"A veces sí", "La la la", "Mi vida entera", "Do you know", "Final de la noche"} {
		if got := FromSheet(line, "Estrofa").Sections[0].Body; got != line {
			t.Errorf("%q became %q", line, got)
		}
	}
}

func TestASheetIsCutIntoSections(t *testing.T) {
	sheet := strings.Join([]string{
		"Cejilla en 2",
		"",
		"Intro: Am  G  F  E (x2)",
		"",
		"Am" + strings.Repeat(" ", 16) + "G",
		"Esto es una letra inventada",
		"",
		"",
		"",
		"ESTRIBILLO:",
		"C        G",
		"La la la la la",
		"#hashtag en la letra",
		"",
		"e|-----0---|",
		"B|---1-----|",
		"G|-2-------|",
		"",
		"[Puente]",
		"Final de la prueba",
	}, "\n")

	p := FromSheet(sheet, "Estrofa")
	if p.Capo != "2" {
		t.Errorf("capo = %q, want 2", p.Capo)
	}
	want := []Part{
		{Label: "Intro", Body: "[Am] [G] [F] [E] [x2]"},
		{Label: "Estrofa", Body: "[Am]Esto es una letra [G]inventada"},
		{Label: "Estribillo", Body: "[C]La la la [G]la la\n\\#hashtag en la letra\n\n```tab\ne|-----0---|\nB|---1-----|\nG|-2-------|\n```"},
		{Label: "Puente", Body: "Final de la prueba"},
	}
	if !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("sections:\n%#v\nwant\n%#v", p.Sections, want)
	}
}

// The verse after a chorus is a verse, not more chorus. A heading already in
// the format runs to the next one, blank lines and all.
func TestAStanzaAfterANamedSectionIsAVerse(t *testing.T) {
	p := FromSheet("primera\n\nCoro:\nla la\n\nsegunda\nmás segunda\n\nCORO\nla la", "Estrofa")
	want := []Part{
		{Body: "primera"},
		{Label: "Coro", Body: "la la"},
		{Label: "Estrofa", Body: "segunda\nmás segunda"},
		{Label: "Coro", Body: "la la"},
	}
	if !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("sections:\n%#v\nwant\n%#v", p.Sections, want)
	}

	p = FromSheet("## Estrofa 1\nuna\n\notra", "Estrofa")
	if want := []Part{{Label: "Estrofa 1", Body: "una\n\notra"}}; !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("a heading in the format was split: %#v", p.Sections)
	}
}

// A sheet with a blank line after every line does not come apart into a
// section per line: the verse that follows the chorus is one verse.
func TestADoubleSpacedSheetStaysInOnePiece(t *testing.T) {
	p := FromSheet("Coro:\nla la\n\nAm\nprimera\n\nG\nsegunda\n\ntercera", "Estrofa")
	want := []Part{
		{Label: "Coro", Body: "la la"},
		{Label: "Estrofa", Body: "[Am]primera\n\n[G]segunda\n\ntercera"},
	}
	if !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("sections:\n%#v\nwant\n%#v", p.Sections, want)
	}
}

// What a sheet draws around and between its parts is decoration: the name in
// the middle of a row of dashes is the name, and a row of dashes on its own is
// a gap.
func TestASheetSeesPastDecoration(t *testing.T) {
	p := FromSheet("-----estribillo-----\nla la\n----------\nletra", "Estrofa")
	want := []Part{{Label: "Estribillo", Body: "la la"}, {Label: "Estrofa", Body: "letra"}}
	if !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("sections:\n%#v\nwant\n%#v", p.Sections, want)
	}
}

// Tablature written without string names or bars is still tablature, once
// there are enough rows of it to be sure.
func TestATabWithoutBarsIsATab(t *testing.T) {
	p := FromSheet("-6-6-5-3--\n--------5-\n----------\n\nMIm\nletra", "Estrofa")
	want := "```tab\n-6-6-5-3--\n--------5-\n----------\n```\n\n[Em]letra"
	if len(p.Sections) != 1 || p.Sections[0].Body != want {
		t.Errorf("sections = %#v\nwant body %q", p.Sections, want)
	}
}

// A sheet in capitals comes in in sentence case, chords and all left alone.
func TestAShoutingSheetIsBroughtDown(t *testing.T) {
	p := FromSheet("Am      G\nNO SÉ SI VENDRÁS. ¿Y TÚ?\nYO ESPERO AQUÍ, EN LA PUERTA", "Estrofa")
	want := "[Am]No sé si [G]vendrás. ¿Y tú?\nYo espero aquí, en la puerta"
	if got := p.Sections[0].Body; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A sheet in ordinary case is not touched, capitals and all.
	if got := FromSheet("Am\nVoy a CASA de Ana", "Estrofa").Sections[0].Body; got != "[Am]Voy a CASA de Ana" {
		t.Errorf("an ordinary sheet was changed: %q", got)
	}
}

// What comes out is a song once it has a title: it reads back, and the chords
// in it are the chords the sheet had.
func TestASheetBecomesASong(t *testing.T) {
	p := FromSheet("Intro: Lam Sol\n\nAm         F\nprimera línea\nC/E       G7sus4\nsegunda línea", "Estrofa")
	p.Title, p.Artist = "Prueba", "Nadie"

	s, err := Parse(Put(p))
	if err != nil {
		t.Fatalf("%v\n%s", err, Put(p))
	}
	var got []string
	s.MapChords(func(c chord.Chord) chord.Chord {
		got = append(got, c.String())
		return c
	})
	if want := []string{"Am", "G", "Am", "F", "C/E", "G7sus4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("chords = %v, want %v", got, want)
	}
}

func TestIsFile(t *testing.T) {
	for text, want := range map[string]bool{
		"---\ntitle: Prueba\n---\n": true,
		"\ufeff---\r\ntitle: x\r\n": true,
		"--- \ntitle: x\n":          true,
		"Am   G\nletra\n":           false,
		"\n---\ntitle: x\n---\n":    false,
		"":                          false,
	} {
		if got := IsFile([]byte(text)); got != want {
			t.Errorf("IsFile(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestATypedChordIsReadGenerously(t *testing.T) {
	for typed, want := range map[string]string{
		"Am":      "Am",
		" F#m7 ":  "F#m7",
		"am":      "Am",
		"am/g":    "Am/G",
		"c6/9":    "C6/9",
		"bb":      "Bb",
		"Lam":     "Am",
		"lam":     "Am",
		"LAm":     "Am",
		"sol7":    "G7",
		"do#m":    "C#m",
		"sib":     "Bb",
		"mi/sol#": "E/G#",
		"Fadd9":   "Fadd9",
		"fa":      "F",
	} {
		c, ok := TypedChord(typed)
		if !ok || c.String() != want {
			t.Errorf("%q was read as %q (%v), want %q", typed, c.String(), ok, want)
		}
	}
	for _, typed := range []string{"", "x2", "bis", "N.C.", "let ring", "solo", "coda", "dim", "do re mi", "H7"} {
		if c, ok := TypedChord(typed); ok {
			t.Errorf("%q was read as the chord %q", typed, c.String())
		}
	}
}
