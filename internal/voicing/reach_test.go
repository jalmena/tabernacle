package voicing

import "testing"

// The hardest band has to be reachable, or it is a word nobody will ever see.
// It used to begin past the ninety-ninth percentile of everything the generator
// produces, which is what this guards against.
func TestTheHardestBandCanBeReached(t *testing.T) {
	// Six strings, every finger as far from every other as the neck allows.
	awful := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 5, 2, 9, 3, 12}})
	if got := awful.Level(); got != Inhuman {
		t.Errorf("a deliberate mess costs %d and is level %d, want Inhuman", awful.Cost, got)
	}

	// And an ordinary hard chord must not be: the band means something only if
	// it is rare.
	fine := Score(Voicing{Tuning: Standard(), Frets: []Fret{1, 3, 3, 2, 1, 1}})
	if fine.Level() >= Extreme {
		t.Errorf("F costs %d and is level %d, which is too harsh", fine.Cost, fine.Level())
	}
}
