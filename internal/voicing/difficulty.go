package voicing

import "math"

// How hard a shape is, which is what the best placement of the hand costs.
//
// strain is what the hand is asked to do and nothing about where on the neck it
// happens; Level reads it. cost is strain plus two editorial preferences and is
// only ever a sort key.

// strain is what the shape asks of the hand.
func strain(v Voicing) int {
	// A shape with no fingering asks for more fingers than a hand has. All of
	// these are inhuman, so they start above the last band, and the only thing
	// left to say is which is further out of reach: a shape two fingers short of
	// a hand ranks past one short by a single finger.
	//
	// It is worked out here rather than in Level so that the number shown beside
	// the word is the number the word came from. They disagreed for a while, and
	// the page said "Inhuman (64)" — a band the number is nowhere near.
	if !holdable(v) {
		return levels[len(levels)-1] + 1 + 40*shortBy(v)
	}

	c, ok := place(v)
	if !ok {
		// A fingering exists on paper but the hand will not go there: fingers
		// would have to cross, or one would have to be in two places. Inhuman,
		// and further out than merely running short of fingers.
		return levels[len(levels)-1] + 1 + 40*max(1, shortBy(v)+1)
	}

	return scored(v, c)
}

// scored turns what a placement cost into the number the levels are banded on.
//
// A muted string with sounded strings either side has to be damped on purpose,
// usually with the underside of a finger already busy. It is work the hand does
// that no fingertip position can show, so it is added here rather than in the
// placement.
func scored(v Voicing, placed float64) int {
	placed += float64(mutedInside(v.Frets)) * damping
	return int(math.Round(placed * gain))
}

// cost is the sort key for a list of shapes: how hard it is, plus what a chord
// reference would rather print first.
func cost(v Voicing) int {
	return strain(v) + 3*v.Position + 6*silent(v.Frets)
}
