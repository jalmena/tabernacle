package voicing

import "testing"

// The hardest band has to be reachable, or it is a word nobody will ever see.
// It used to begin past the ninety-ninth percentile of everything the generator
// produces, which is what this guards against.
func TestTheHardestBandCanBeReached(t *testing.T) {
	// Six strings on six different frets, spread the length of the neck. No
	// hand has six fingers, so this is not a hard shape but an impossible one,
	// and it has to score above every shape that can actually be held.
	awful := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 5, 2, 9, 3, 12}})
	if got := awful.Level(); got != Inhuman {
		t.Errorf("a deliberate mess costs %d and is level %d, want Inhuman", awful.Cost, got)
	}

	// Anything with no fingering lands there whatever its other terms come to,
	// including a shape whose notes are otherwise unremarkable: six strings on
	// six frets in a row is one finger too few, and that is the whole point.
	crowded := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 2, 3, 4, 5, 6}})
	if len(crowded.Fingers) != 0 {
		t.Fatal("six separate frets should need six fingers and find none")
	}
	if got := crowded.Level(); got != Inhuman {
		t.Errorf("six fingers' worth costs %d and is level %d, want Inhuman", crowded.Cost, got)
	}

	// And an ordinary hard chord must not be: the band means something only if
	// it is rare.
	fine := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 3, 3, 2, 1, 1}})
	if fine.Level() >= Extreme {
		t.Errorf("F costs %d and is level %d, which is too harsh", fine.Cost, fine.Level())
	}
}

// The number shown beside the word has to be a number in that word's band.
// They disagreed: a shape with no fingering was floored into the last band by
// Level while Strain went on reporting the terms it could measure, so the page
// said "Inhuman (64)" — a number two bands below the word next to it.
func TestTheNumberAgreesWithTheWord(t *testing.T) {
	for _, frets := range [][]Fret{
		{1, 2, 3, 4, 5, 6},  // six frets, six fingers, one too few
		{1, 5, 2, 9, 3, 12}, // and the same spread the length of the neck
		{1, 3, 3, 2, 1, 1},  // F, which is holdable and must be unaffected
		{Muted, Muted, 0, 2, 3, 2},
	} {
		v := Score(Voicing{Tuning: Standard(), Frets: frets})
		if got, want := levelFor(v.Strain()), v.Level(); got != want {
			t.Errorf("%s shows %d, which is level %d, beside the word for level %d",
				v.Grip(), v.Strain(), got, want)
		}
	}
}

// Among shapes no hand can hold, the one further out of reach ranks past the
// other. They all used to land on the same floor.
func TestBeingShorterOfFingersIsWorse(t *testing.T) {
	// Five fingers' worth: four separate frets and a barre under them.
	byOne := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 1, 2, 3, 4, 5}})
	// Six, with nothing a barre can cover.
	byTwo := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 2, 3, 4, 5, 6}})

	for _, v := range []Voicing{byOne, byTwo} {
		if v.Level() != Inhuman {
			t.Fatalf("%s is level %d, want Inhuman", v.Grip(), v.Level())
		}
	}
	if byOne.Strain() >= byTwo.Strain() {
		t.Errorf("one finger short scores %d and two fingers short %d",
			byOne.Strain(), byTwo.Strain())
	}
}
