package chord

import "testing"

func mustParse(t *testing.T, s string) Chord {
	t.Helper()
	c, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return c
}

func TestTransposeChord(t *testing.T) {
	tests := []struct {
		in        string
		semitones int
		spelling  Spelling
		want      string
	}{
		// Quality, extensions and slash bass must survive the move.
		{"C", 2, Sharps, "D"},
		{"Cm", 2, Sharps, "Dm"},
		{"C7", 2, Sharps, "D7"},
		{"Cmaj7", 2, Sharps, "Dmaj7"},
		{"C/E", 2, Sharps, "D/F#"},
		{"Cm7b5", 2, Sharps, "Dm7b5"},
		{"Cadd9", 2, Sharps, "Dadd9"},
		{"C6/9", 2, Sharps, "D6/9"},
		{"Cm(maj7)", 2, Sharps, "Dm(maj7)"},
		{"Am/G", 3, Flats, "Cm/Bb"},

		// The same move, spelled both ways.
		{"C", 1, Sharps, "C#"},
		{"C", 1, Flats, "Db"},
		{"A", 1, Flats, "Bb"},
		{"A", 1, Sharps, "A#"},

		// Wrapping around the octave, in both directions.
		{"B", 1, Sharps, "C"},
		{"C", -1, Flats, "B"},
		{"G", 7, Sharps, "D"},
		{"G", -5, Sharps, "D"},
	}
	for _, tt := range tests {
		got := mustParse(t, tt.in).Transpose(tt.semitones, tt.spelling).String()
		if got != tt.want {
			t.Errorf("Parse(%q).Transpose(%d, %v) = %q, want %q",
				tt.in, tt.semitones, tt.spelling, got, tt.want)
		}
	}
}

// TestTransposeIdentity pins down that a no-op transposition does not respell.
// Without it, opening and saving a song in Gb would silently rewrite every
// chord as F# and produce a diff the user never asked for.
func TestTransposeIdentity(t *testing.T) {
	for _, s := range []string{"Gb", "F#", "A#m7", "Bbm7", "Cb", "Db/Ab"} {
		c := mustParse(t, s)
		for _, n := range []int{0, 12, -12, 24} {
			for _, sp := range []Spelling{Sharps, Flats} {
				if got := c.Transpose(n, sp).String(); got != s {
					t.Errorf("Parse(%q).Transpose(%d, %v) = %q, want unchanged",
						s, n, sp, got)
				}
			}
		}
	}
}

func TestSpellingForKey(t *testing.T) {
	tests := map[string]Spelling{
		"C": Sharps, "G": Sharps, "D": Sharps, "A": Sharps, "E": Sharps, "B": Sharps,
		"F": Flats, "Bb": Flats, "Eb": Flats, "Ab": Flats, "Db": Flats, "Gb": Flats,
		"F#": Sharps, "C#": Sharps,
		"Am": Sharps, "Em": Sharps, "Bm": Sharps, "F#m": Sharps, "C#m": Sharps,
		"Dm": Flats, "Gm": Flats, "Cm": Flats, "Fm": Flats, "Bbm": Flats, "Ebm": Flats,
	}
	for in, want := range tests {
		if got := SpellingForKey(mustParse(t, in)); got != want {
			t.Errorf("SpellingForKey(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestTransposeKey checks the destination keys a guitarist expects to read,
// not merely the right pitches.
func TestTransposeKey(t *testing.T) {
	tests := []struct {
		key       string
		semitones int
		want      string
		spelling  Spelling
	}{
		{"C", 2, "D", Sharps},
		{"C", 1, "Db", Flats},
		{"E", 1, "F", Flats},   // F major is written with a flat
		{"G", -2, "F", Flats},  // so is the key you land in going down a tone
		{"Am", 3, "Cm", Flats}, // C minor has three flats
		{"Am", 1, "Bbm", Flats},
		{"Bb", 1, "B", Sharps}, // B major beats Cb major on any guitar
		{"D", -2, "C", Sharps},
		{"Em", 2, "F#m", Sharps},
		{"F#m", -1, "Fm", Flats},
		{"Gb", 0, "Gb", Flats}, // no move, no respelling
		{"C#m", 0, "C#m", Sharps},
		{"Am7", 3, "Cm7", Flats}, // the suffix rides along
	}
	for _, tt := range tests {
		gotKey, gotSp := TransposeKey(mustParse(t, tt.key), tt.semitones)
		if got := gotKey.String(); got != tt.want {
			t.Errorf("TransposeKey(%q, %d) key = %q, want %q",
				tt.key, tt.semitones, got, tt.want)
		}
		if gotSp != tt.spelling {
			t.Errorf("TransposeKey(%q, %d) spelling = %v, want %v",
				tt.key, tt.semitones, gotSp, tt.spelling)
		}
	}
}

// TestTransposeKeyDrivesChords is the end-to-end property that matters: pick
// the spelling once from the destination key, and a whole sheet comes out
// internally consistent.
func TestTransposeKeyDrivesChords(t *testing.T) {
	// A ii-V-I in E, moved up a semitone, has to read as F major: F, Bb, C.
	sheet := []string{"E", "A", "B", "C#m"}
	want := []string{"F", "Bb", "C", "Dm"}

	_, sp := TransposeKey(mustParse(t, "E"), 1)
	for i, s := range sheet {
		if got := mustParse(t, s).Transpose(1, sp).String(); got != want[i] {
			t.Errorf("%q transposed +1 in E = %q, want %q", s, got, want[i])
		}
	}
}
