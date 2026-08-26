package voicing

import (
	"testing"
)

// Where the levels are anchored now lives in testdata/judgements.json, checked
// by TestTheChordsLandWhereThePlayerPutThem. A list here as well was a second
// opinion nobody had asked for, and the two had already drifted apart: this one
// said B and Bb were advanced while the player who was asked put every barre in
// one band.

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
// to work out the fingering too, or there is nothing for the hand to be placed
// on and every drawing comes back comfortable.
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
	for _, v := range []Voicing{easy, hard} {
		if len(v.Fingers) != len(v.Frets) {
			t.Fatalf("%s came back with no fingering to place a hand on", v.Grip())
		}
	}
	if _, ok := place(easy); !ok {
		t.Error("E major cannot be held, which cannot be right")
	}
	if easy.Level() != Trivial && easy.Level() != Easy {
		t.Errorf("E major came out level %d", easy.Level())
	}
}

// That the same barre is harder at the first fret than at the second is one of
// the ratios in testdata/judgements.json, where the number a guitarist actually
// quoted for it — half again — is written down instead of only its direction.

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
