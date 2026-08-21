package song

import (
	"strings"
	"testing"
)

func TestKindForLabel(t *testing.T) {
	tests := map[string]Kind{
		"Intro":        KindIntro,
		"Verse":        KindVerse,
		"Verse 1":      KindVerse,
		"VERSE II":     KindVerse,
		"verse-2":      KindVerse,
		"Estrofa 2":    KindVerse,
		"Pre-Chorus":   KindPreChorus,
		"pre chorus":   KindPreChorus,
		"Chorus":       KindChorus,
		"Chorus (x2)":  KindChorus,
		"Estribillo":   KindChorus,
		"Coro":         KindChorus,
		"Bridge":       KindBridge,
		"Puente":       KindBridge,
		"Solo":         KindSolo,
		"Instrumental": KindSolo,
		"Outro":        KindOutro,
		"Coda":         KindOutro,
		"Tab":          KindTab,
		"Tablatura":    KindTab,
		"Introducción": KindIntro,
		"Whatever":     KindOther,
		"":             KindOther,
	}
	for label, want := range tests {
		if got := KindForLabel(label); got != want {
			t.Errorf("KindForLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func parseSong(t *testing.T, src string) Song {
	t.Helper()
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

// chordsOf flattens the chord symbols of a song in reading order.
func chordsOf(s Song) []string {
	var out []string
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			for _, tok := range ln.Tokens {
				if tok.Kind == TokenChord {
					out = append(out, tok.Chord.String())
				}
			}
		}
	}
	return out
}

func TestTransposeSong(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Verse 1\n[E]one [A]two [B]three [C#m]four [A/C#]five [x2]\n"
	s := parseSong(t, src)

	up := s.Transpose(1)
	if up.Key != "F" {
		t.Errorf("key = %q, want %q", up.Key, "F")
	}
	// F major reads in flats, so every chord on the page has to agree.
	want := "F Bb C Dm Bb/D"
	if got := strings.Join(chordsOf(up), " "); got != want {
		t.Errorf("chords = %q, want %q", got, want)
	}
	// The annotation must not have been touched.
	if !strings.Contains(string(mustMarshal(t, up)), "[x2]") {
		t.Error("annotation did not survive transposition")
	}
	// And the original must be untouched.
	if got := strings.Join(chordsOf(s), " "); got != "E A B C#m A/C#" {
		t.Errorf("original mutated: %q", got)
	}
}

func TestTransposeNoKeyUsesFirstChord(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Verse 1\n[Am]one [Dm]two [E]three\n"
	s := parseSong(t, src)

	// Am up three semitones lands in Cm, which is written with flats.
	up := s.Transpose(3)
	if got, want := strings.Join(chordsOf(up), " "), "Cm Fm G"; got != want {
		t.Errorf("chords = %q, want %q", got, want)
	}
	if up.Key != "" {
		t.Errorf("key = %q, want it left empty", up.Key)
	}
}

func TestTransposeIdentity(t *testing.T) {
	src := "---\ntitle: T\nkey: Gb\n---\n\n## Verse 1\n[Gb]one [Db]two [Cb]three\n"
	s := parseSong(t, src)
	for _, n := range []int{0, 12, -12} {
		got := s.Transpose(n)
		if key := got.Key; key != "Gb" {
			t.Errorf("Transpose(%d) key = %q, want Gb", n, key)
		}
		if c := strings.Join(chordsOf(got), " "); c != "Gb Db Cb" {
			t.Errorf("Transpose(%d) chords = %q, want them unchanged", n, c)
		}
	}
}

func TestTransposeLeavesCapoAlone(t *testing.T) {
	src := "---\ntitle: T\nkey: G\ncapo: 2\n---\n\n## Verse 1\n[G]one\n"
	s := parseSong(t, src).Transpose(2)
	if s.Capo != 2 {
		t.Errorf("capo = %d, want 2", s.Capo)
	}
}

func mustMarshal(t *testing.T, s Song) []byte {
	t.Helper()
	b, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

// A solo has to move with the chords above it. A sheet whose verse is in F and
// whose tab is still in E is worse than one with no tab at all.
func TestTransposeMovesTablature(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Verse 1\n[E]one\n\n## Solo\n```tab\n" +
		"e|---5---7---|\nB|---5---7---|\n```\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out, rep := s.TransposeReport(2)
	if rep.TabsShifted != 1 {
		t.Errorf("report = %+v, want one tab shifted", rep)
	}
	if rep.Clamped() {
		t.Errorf("report = %+v, want no clamping", rep)
	}

	var tabText string
	for _, sec := range out.Sections {
		for _, ln := range sec.Lines {
			if ln.Kind == LineVerbatim {
				tabText = ln.Text
			}
		}
	}
	if !strings.Contains(tabText, "7---9") {
		t.Errorf("the tab did not move up two frets:\n%s", tabText)
	}
	// And the chords moved too, in the same direction.
	if out.Key != "F#" {
		t.Errorf("key = %q, want F#", out.Key)
	}
}

// Going below an open string is impossible, so the block stays put and says so.
func TestTransposeClampsTablatureAtTheOpenString(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Solo\n```tab\ne|---0---3---|\nB|---0---1---|\n```\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	original := s.Sections[0].Lines[0].Text

	out, rep := s.TransposeReport(-2)
	if rep.TabsTooLow != 1 {
		t.Errorf("report = %+v, want one tab refused for being too low", rep)
	}
	if !rep.Clamped() {
		t.Error("Clamped() is false even though a tab was left behind")
	}
	if got := out.Sections[0].Lines[0].Text; got != original {
		t.Errorf("the tab was altered despite being refused:\n%s", got)
	}
	// The chords still move: only the tablature is stuck.
	if out.Key != "D" {
		t.Errorf("key = %q, want D", out.Key)
	}
}

func TestTransposeClampsTablatureAtTheEndOfTheNeck(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Solo\n```tab\ne|---22---|\nB|---20---|\n```\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out, rep := s.TransposeReport(5)
	if rep.TabsTooHigh != 1 {
		t.Errorf("report = %+v, want one tab refused for running off the neck", rep)
	}
	if !strings.Contains(out.Sections[0].Lines[0].Text, "22") {
		t.Error("the tab was altered despite being refused")
	}
}

// A verbatim block that is not tablature is never touched, whatever the shift.
func TestTransposeLeavesNonTabBlocksAlone(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Notes\n```\nPlay the intro twice.\n```\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out, rep := s.TransposeReport(3)
	if rep.TabsShifted != 0 || rep.Clamped() {
		t.Errorf("report = %+v, want nothing done to a prose block", rep)
	}
	if !strings.Contains(out.Sections[0].Lines[0].Text, "Play the intro twice.") {
		t.Error("a prose block was modified")
	}
}

// Transpose keeps its old signature, so nothing that only wanted the song had to
// change.
func TestTransposeWrapperStillWorks(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Solo\n```tab\ne|---5---|\n```\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Transpose(2); !strings.Contains(got.Sections[0].Lines[0].Text, "7") {
		t.Error("the wrapper did not move the tab")
	}
}
