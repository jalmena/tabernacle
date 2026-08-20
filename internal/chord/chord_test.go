package chord

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in     string
		root   Note
		suffix string
		bass   Note
		hasBas bool
	}{
		{in: "C", root: Note{'C', 0}},
		{in: "Am", root: Note{'A', 0}, suffix: "m"},
		{in: "F#m", root: Note{'F', 1}, suffix: "m"},
		{in: "Bb", root: Note{'B', -1}},
		{in: "Ab", root: Note{'A', -1}},
		{in: "C7", root: Note{'C', 0}, suffix: "7"},
		{in: "Cmaj7", root: Note{'C', 0}, suffix: "maj7"},
		{in: "CM7", root: Note{'C', 0}, suffix: "M7"},
		{in: "Cadd9", root: Note{'C', 0}, suffix: "add9"},
		{in: "Csus4", root: Note{'C', 0}, suffix: "sus4"},
		{in: "C7sus4", root: Note{'C', 0}, suffix: "7sus4"},
		{in: "Cmin7", root: Note{'C', 0}, suffix: "min7"},
		{in: "Cm7b5", root: Note{'C', 0}, suffix: "m7b5"},
		{in: "Cdim7", root: Note{'C', 0}, suffix: "dim7"},
		{in: "C°", root: Note{'C', 0}, suffix: "°"},
		{in: "Caug", root: Note{'C', 0}, suffix: "aug"},
		{in: "C+", root: Note{'C', 0}, suffix: "+"},
		{in: "C7(b9)", root: Note{'C', 0}, suffix: "7(b9)"},
		{in: "Cmaj7#11", root: Note{'C', 0}, suffix: "maj7#11"},
		{in: "Cm(maj7)", root: Note{'C', 0}, suffix: "m(maj7)"},
		{in: "C5", root: Note{'C', 0}, suffix: "5"},
		{in: "Bb5", root: Note{'B', -1}, suffix: "5"},
		{in: "Dno3", root: Note{'D', 0}, suffix: "no3"},
		{in: "C13", root: Note{'C', 0}, suffix: "13"},
		{in: "C11", root: Note{'C', 0}, suffix: "11"},
		{in: "Cm7♭5", root: Note{'C', 0}, suffix: "m7♭5"},

		// Slash chords, and the suffix that merely looks like one.
		{in: "C/E", root: Note{'C', 0}, suffix: "", bass: Note{'E', 0}, hasBas: true},
		{in: "Am/G", root: Note{'A', 0}, suffix: "m", bass: Note{'G', 0}, hasBas: true},
		{in: "D/F#", root: Note{'D', 0}, suffix: "", bass: Note{'F', 1}, hasBas: true},
		{in: "Fmaj7/A", root: Note{'F', 0}, suffix: "maj7", bass: Note{'A', 0}, hasBas: true},
		{in: "C/Bb", root: Note{'C', 0}, suffix: "", bass: Note{'B', -1}, hasBas: true},
		{in: "C6/9", root: Note{'C', 0}, suffix: "6/9"},
		{in: "C6/9/E", root: Note{'C', 0}, suffix: "6/9", bass: Note{'E', 0}, hasBas: true},

		// Surrounding whitespace is forgiven; sheets are typed by humans.
		{in: "  Dm  ", root: Note{'D', 0}, suffix: "m"},
	}

	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tt.in, err)
			continue
		}
		want := Chord{Root: tt.root, Suffix: tt.suffix, Bass: tt.bass, HasBass: tt.hasBas}
		if got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, want)
		}
	}
}

// TestParseRejectsText is the test that earns the suffix whitelist its keep. A
// permissive parser turns "[Chorus]" into C with the suffix "horus", which a
// transposition then renders as "Dhorus".
func TestParseRejectsText(t *testing.T) {
	notChords := []string{
		"", "   ",
		"Chorus", "chorus", "Verse", "Verse 1", "Bridge", "Intro", "Outro",
		"Solo", "Coda", "Break", "Bass", "Fine", "End", "Fade out", "Capo 2",
		"x2", "N.C.", "NC", "Guitar", "Dad", "Bam", "H", "Do", "Sib",
		"am", "bm", "c", // lowercase roots are deliberately not accepted
		"C 7", "C#Chorus",
	}
	for _, s := range notChords {
		if got, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = %v, want error", s, got)
		} else if !errors.Is(err, ErrNotAChord) {
			t.Errorf("Parse(%q) error = %v, want ErrNotAChord", s, err)
		}
	}
}

// TestFormatIsLossless guards the property the file writer depends on: a chord
// read from a file is written back exactly as it was typed.
func TestFormatIsLossless(t *testing.T) {
	symbols := []string{
		"C", "Am", "F#m7", "Bb", "Cmaj7", "CM7", "Cm7b5", "C6/9", "C/E",
		"Ab/Eb", "Dsus2", "G7sus4", "C°", "Cø7", "CΔ7", "Cm7♭5", "Cadd9",
	}
	for _, s := range symbols {
		c, err := Parse(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		if got := c.String(); got != s {
			t.Errorf("Parse(%q).String() = %q, want %q", s, got, s)
		}
	}
}

func TestIsMinor(t *testing.T) {
	tests := map[string]bool{
		"C": false, "Cm": true, "Cm7": true, "Cmin": true, "Cmaj7": false,
		"CM7": false, "Cmmaj7": true, "C-": true, "Cdim": false, "C7": false,
		"Am": true, "Asus4": false,
	}
	for in, want := range tests {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := c.IsMinor(); got != want {
			t.Errorf("Parse(%q).IsMinor() = %v, want %v", in, got, want)
		}
	}
}

func TestPitchClass(t *testing.T) {
	tests := map[string]int{
		"C": 0, "C#": 1, "Db": 1, "D": 2, "E": 4, "F": 5, "F#": 6,
		"Gb": 6, "G": 7, "A": 9, "Bb": 10, "B": 11, "Cb": 11, "B#": 0,
	}
	for in, want := range tests {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := c.Root.PitchClass(); got != want {
			t.Errorf("%q pitch class = %d, want %d", in, got, want)
		}
	}
	if got := (Note{}).PitchClass(); got != 0 {
		t.Errorf("zero Note pitch class = %d, want 0", got)
	}
}
