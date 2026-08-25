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
