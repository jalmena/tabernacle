package voicing

import (
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

// The bands are only worth anything if the chords a player already has an
// opinion about land where that opinion says they should.
func TestTheLevelsLandWhereAPlayerWouldPutThem(t *testing.T) {
	for _, tc := range []struct {
		sym  string
		want Level
	}{
		// Two and three fingers, and no barre: the first chords anybody holds.
		{"Em7", Trivial},
		{"Em", Trivial},
		{"E7", Trivial},
		// The rest of the open chords.
		{"E", Easy},
		{"A", Easy},
		{"G", Easy},
		{"C", Easy},
		{"D", Easy},
		{"Am", Easy},
		// The barres everybody fights and then masters.
		{"F", Moderate},
		{"Bm", Moderate},
		// B is the A-shape barre, which is a step past the other two: the index
		// holds the sixth string down while three more fingers crowd onto one
		// fret. It is the chord that stops people who have already got F.
		{"B", Advanced},
	} {
		t.Run(tc.sym, func(t *testing.T) {
			c, err := chord.Parse(tc.sym)
			if err != nil {
				t.Fatal(err)
			}
			g, ok := Generate(c, GenerateOptions{})
			if !ok || len(g.Root.Shapes) == 0 {
				t.Fatalf("%s has no shape", tc.sym)
			}
			v := g.Root.Shapes[0]
			if got := v.Level(); got != tc.want {
				t.Errorf("%s (%s, cost %d) is level %d, want %d",
					tc.sym, v.Grip(), v.Cost, got, tc.want)
			}
		})
	}
}

// The scale has to be a scale: harder shapes never come out easier.
func TestTheLevelsNeverGoBackwards(t *testing.T) {
	for cost := 0; cost < 400; cost++ {
		a := Voicing{Cost: cost}.Level()
		b := Voicing{Cost: cost + 1}.Level()
		if b < a {
			t.Fatalf("cost %d is level %d but %d is level %d", cost, a, cost+1, b)
		}
	}
	if got := (Voicing{Cost: 10_000}).Level(); got != Inhuman {
		t.Errorf("an absurd cost is level %d, want Inhuman", got)
	}
}

// A shape drawn on the neck arrives with frets and nothing else. Scoring it has
// to work out the fingering too, or posture measures nothing and every drawing
// comes back comfortable.
func TestScoringADrawnShapeWorksOutTheFingers(t *testing.T) {
	drawn := func(grip string) Voicing {
		frets, ok := ParseGrip(grip)
		if !ok {
			t.Fatalf("cannot read %q", grip)
		}
		return Score(Voicing{Tuning: Standard(), Frets: frets})
	}

	easy := drawn("0-2-2-1-0-0") // E major, three fingers, no stretch
	hard := drawn("x-2-1-4-0-2") // the ring finger folded back under the little
	if easy.Cost >= hard.Cost {
		t.Errorf("E major costs %d and a contortion costs %d", easy.Cost, hard.Cost)
	}
	if posture(easy) != 0 {
		t.Errorf("E major strains the hand by %d, want 0", posture(easy))
	}
	if posture(hard) == 0 {
		t.Errorf("the contortion strains the hand by 0, which cannot be right")
	}
	if easy.Level() != Trivial && easy.Level() != Easy {
		t.Errorf("E major came out level %d", easy.Level())
	}
}

// The same barre is harder the nearer the nut it is laid: the strings are
// highest and tightest at the first fret. It is the whole difference between F
// and F#, which are one shape and one fret apart.
func TestTheSameBarreIsHarderAtTheFirstFret(t *testing.T) {
	first := func(sym string) Voicing {
		t.Helper()
		c, err := chord.Parse(sym)
		if err != nil {
			t.Fatal(err)
		}
		g, ok := Generate(c, GenerateOptions{})
		if !ok || len(g.Root.Shapes) == 0 {
			t.Fatalf("%s has no shape", sym)
		}
		return g.Root.Shapes[0]
	}

	for _, tc := range []struct{ nut, next string }{
		{"F", "F#"}, // the E shape, at the first fret and at the second
		{"Bb", "B"}, // the A shape, likewise
	} {
		a, b := first(tc.nut), first(tc.next)
		if a.Position != 1 || b.Position != 2 {
			t.Fatalf("%s is at fret %d and %s at fret %d; this test needs 1 and 2",
				tc.nut, a.Position, tc.next, b.Position)
		}
		if a.Cost <= b.Cost {
			t.Errorf("%s at the first fret costs %d and %s at the second costs %d",
				tc.nut, a.Cost, tc.next, b.Cost)
		}
	}
}
