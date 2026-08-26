package voicing

import "math"

// The neck a hand is placed on, in millimetres.
//
// Everything about how hard a shape is starts here, because a fretboard is not
// evenly spaced. Frets fall where the string divides, so they crowd together as
// they climb: the first is 36.4 mm wide and the twelfth 19.3. Knuckles do not
// move — they sit about 21 mm apart whatever fret the hand is over. Those two
// numbers meeting around the eleventh fret is why one finger per fret is a
// stretch at the nut and the shape of the hand higher up, and it is the reason
// this file exists instead of a table of weights.
//
// The measurements are a common steel-string: a 25.5" scale, a 43 mm nut. They
// are the sort of thing that can be checked against an actual guitar, which is
// the point of writing them down as lengths rather than as points.
const (
	scale = 647.7 // the vibrating length of an open string, 25.5"

	// How far apart the outer strings sit, centre to centre. The neck widens
	// toward the body, so a shape across the strings is wider up the neck.
	nutSpread     = 35.0
	twelfthSpread = 51.0

	// The clearance under an open string over the fret named, which is how far
	// a fingertip has to press it before it sounds. It grows down the neck.
	actionOne    = 0.55
	actionTwelve = 2.2

	// How close to the fret a fingertip wants to land. A note sounds cleanest
	// with the finger right behind the fret; further back it buzzes or needs
	// more force, and this is the distance the hand aims for.
	behindFret = 4.0
)

// fretAt is where fret n crosses the strings, measured from the nut.
//
// Each fret shortens the string by one twelfth of an octave, which is where the
// crowding comes from and why it is a curve rather than a spacing.
func fretAt(n int) float64 {
	if n <= 0 {
		return 0
	}
	return scale * (1 - math.Pow(2, -float64(n)/12))
}

// cellOf is the stretch of board a fingertip may land on to sound fret f: from
// the fret behind it up to the fret itself.
//
// A note is a range and not a point, which matters. A hand under strain lands
// further back in the cell than it would like, and the cells are widest exactly
// where a guitarist finds the barres hardest.
func cellOf(f Fret) (lo, hi float64) {
	return fretAt(int(f) - 1), fretAt(int(f))
}

// wants is where a fingertip would land on this fret if nothing pulled it
// elsewhere: just behind the fret, or the middle of the cell when the cell is
// too narrow for that to mean anything.
func wants(f Fret) float64 {
	lo, hi := cellOf(f)
	if hi-lo < 2*behindFret {
		return (lo + hi) / 2
	}
	return hi - behindFret
}

// spreadAt is the distance between the outer strings at a point on the neck.
func spreadAt(x float64) float64 {
	t := x / fretAt(12)
	return nutSpread + (twelfthSpread-nutSpread)*t
}

// stringAt is how far string s sits from the middle of the neck at a point
// along it. String 0 is the sixth, the one nearest the player's face.
func stringAt(s int, x float64) float64 {
	w := spreadAt(x)
	return w*float64(s)/5 - w/2
}

// action is how far the string over fret f has to be pressed to sound.
func action(f Fret) float64 {
	one, twelve := fretAt(1), fretAt(12)
	t := (fretAt(int(f)) - one) / (twelve - one)
	return actionOne + (actionTwelve-actionOne)*t
}

// sounding is the length of string left ringing when fret f is held.
//
// It is why the same note is harder to hold cleanly near the nut: a longer
// string swings wider for the same pluck, so it takes a firmer stop before it
// rattles on the fret. The effect is real and it is small — a twentieth between
// the first fret and the second — which is worth knowing, because a guitarist
// puts the difference at half again. The rest of it is charged in hand.go and
// is named there as the fitted number it is.
func sounding(f Fret) float64 { return scale - fretAt(int(f)) }
