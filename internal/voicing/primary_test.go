package voicing

import (
	"fmt"
	"strings"
	"testing"
)

// render turns a shape back into the grid notation the dataset is written in, so
// a failure reads like a chord chart rather than a struct dump.
func render(v Voicing) string {
	parts := make([]string, 0, len(v.Frets))
	for _, f := range v.Frets {
		if f == Muted {
			parts = append(parts, "x")
			continue
		}
		parts = append(parts, fmt.Sprint(int(f)))
	}
	return strings.Join(parts, " ")
}

// The shape offered first is the one most readers will ever look at, so it is
// worth pinning. These are the fingerings a chord reference prints, and a change
// here means the cost function moved — which may be right, but should never be
// accidental.
func TestPrimaryShapes(t *testing.T) {
	tests := []struct{ sym, want string }{
		// Open chords everyone knows.
		{"C", "x 3 2 0 1 0"},
		{"D", "x x 0 2 3 2"},
		{"E", "0 2 2 1 0 0"},
		{"G", "3 2 0 0 3 3"},
		{"A", "x 0 2 2 2 0"},
		{"Am", "x 0 2 2 1 0"},
		{"Dm", "x x 0 2 3 1"},
		{"Em", "0 2 2 0 0 0"},
		{"C7", "x 3 2 3 1 0"},
		{"D7", "x x 0 2 1 2"},
		{"E7", "0 2 0 1 0 0"},
		{"G7", "3 2 0 0 0 1"},
		{"A7", "x 0 2 0 2 0"},
		{"Am7", "x 0 2 0 1 0"},
		{"Em7", "0 2 0 0 0 0"},
		{"Cmaj7", "x 3 2 0 0 0"},
		{"Dmaj7", "x x 0 2 2 2"},
		{"Fmaj7", "x x 3 2 1 0"},

		// The barre chords, at the position a reference prints. Db and Eb are
		// the A shape slid up rather than the four-string grips lower down: a
		// reference prints the movable shape, and it is what a player reaches
		// for in a song that has the rest of its chords up there too.
		{"F", "1 3 3 2 1 1"},
		{"Bb", "x 1 3 3 3 1"},
		{"Db", "x 4 6 6 6 4"},
		{"Eb", "x 6 8 8 8 6"},
		{"Bm", "x 2 4 4 3 2"},
		{"Bm7", "x 2 4 2 3 2"},
		{"B7", "x 2 1 2 0 2"},

		// Slid templates, for roots with no curated shape.
		{"F#m", "2 4 4 2 2 2"},
		{"C#m", "x 4 6 6 5 4"},
		{"Ab", "4 6 6 5 4 4"},

		// Slash chords keep their bass in the lowest voice.
		{"D/F#", "2 0 0 2 3 2"},
		{"C/E", "0 3 2 0 1 0"},
		{"G/B", "x 2 0 0 3 3"},

		// Power chords take the lower of the two shapes.
		{"C5", "x 3 5 x x x"},
		{"E5", "0 2 x x x x"},

		// Suspended and sixth chords.
		{"Dsus4", "x x 0 2 3 3"},
		{"Asus2", "x 0 2 2 0 0"},
		{"A6", "x 0 2 2 2 2"},

		// Half-diminished and diminished, from their movable templates.
		{"Am7b5", "x 0 1 0 1 x"},
		{"Cdim7", "x x 1 2 1 2"},
	}
	for _, tt := range tests {
		t.Run(tt.sym, func(t *testing.T) {
			res := lookup(t, tt.sym)
			if !res.Found() {
				t.Fatalf("%s has no shape", tt.sym)
			}
			if got := render(res.Voicings[0]); got != tt.want {
				t.Errorf("%s primary shape = %q, want %q\n(all: %s)",
					tt.sym, got, tt.want, allShapes(res))
			}
		})
	}
}

func allShapes(res Result) string {
	var parts []string
	for _, v := range res.Voicings {
		parts = append(parts, fmt.Sprintf("%s[%s]", render(v), v.ID()))
	}
	return strings.Join(parts, "  ")
}

// A barre chord should offer somewhere else to play it, since half the reason to
// look at a diagram is to find an easier position.
func TestAlternativePositionsAreOffered(t *testing.T) {
	for _, sym := range []string{"F", "Bb", "Bm", "F#m", "C", "G", "Am7"} {
		res := lookup(t, sym)
		if len(res.Voicings) < 2 {
			t.Errorf("%s offers only one shape: %s", sym, allShapes(res))
		}
		positions := map[int]bool{}
		for _, v := range res.Voicings {
			positions[v.Position] = true
		}
		if len(positions) < 2 {
			t.Errorf("%s offers %d shapes but all at the same position: %s",
				sym, len(res.Voicings), allShapes(res))
		}
	}
}
