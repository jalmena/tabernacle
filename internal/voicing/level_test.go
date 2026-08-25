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
	for s := 0; s < 400; s++ {
		if levelFor(s+1) < levelFor(s) {
			t.Fatalf("strain %d is level %d but %d is level %d", s, levelFor(s), s+1, levelFor(s+1))
		}
	}
	if got := levelFor(10_000); got != Inhuman {
		t.Errorf("an absurd strain is level %d, want Inhuman", got)
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

// The same shape is the same shape wherever it sits. Frets narrow as they
// climb, so if anything a barre is easier high up than low down — it is
// certainly not two bands harder, which is what levelling by cost said.
func TestTheSameShapeKeepsItsLevelUpTheNeck(t *testing.T) {
	shape := func(at int) Voicing {
		return Score(Voicing{Tuning: Standard(), Frets: []Fret{
			Fret(at), Fret(at + 2), Fret(at + 2), Fret(at + 1), Fret(at), Fret(at),
		}})
	}

	want := shape(1).Level()
	for _, at := range []int{2, 3, 5, 7, 9, 12} {
		if got := shape(at).Level(); got != want {
			t.Errorf("the E shape is level %d at the first fret and %d at the %dth",
				want, got, at)
		}
	}

	// And the list still opens where a chord reference does, which is the job
	// the position term is actually for.
	if shape(1).Cost >= shape(5).Cost {
		t.Errorf("the shape at the first fret costs %d and at the fifth %d; a "+
			"reference prints the low one first", shape(1).Cost, shape(5).Cost)
	}
}
