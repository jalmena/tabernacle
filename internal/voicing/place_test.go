package voicing

import "testing"

// What the placement model has to be true of whatever the fitted numbers say.
// These are about the shape of the model rather than its calibration, so they
// hold with any hand and are the tests that would catch the geometry being
// wrong rather than the weights.

func held(t *testing.T, frets string, fingers ...Finger) Voicing {
	t.Helper()
	f, ok := ParseGrip(frets)
	if !ok {
		t.Fatalf("%s: not a grip", frets)
	}
	v := Voicing{Tuning: Standard(), Frets: f, Fingers: make([]Finger, len(f))}
	at := 0
	for i, fr := range f {
		if fr <= 0 {
			continue
		}
		if at >= len(fingers) {
			t.Fatalf("%s: %d fingers given for more notes than that", frets, len(fingers))
		}
		v.Fingers[i] = fingers[at]
		at++
	}
	if at != len(fingers) {
		t.Fatalf("%s: %d fingers given, %d notes", frets, len(fingers), at)
	}
	v.Position = lowestFret(v.Frets)
	return v
}

// The one rule that replaced four. Fingers lie on the neck in the order they
// grow in, and a fingering that asks otherwise is not expensive, it is refused.
func TestFingersCannotCrossOver(t *testing.T) {
	for _, tc := range []struct {
		what    string
		frets   string
		fingers []Finger
	}{
		{"the index above the middle finger and past it",
			"x-x-2-3-x-x", []Finger{Middle, Index}},
		{"the middle finger above the ring and past it",
			"x-x-3-4-x-x", []Finger{Ring, Middle}},
		{"the ring above the little finger and past it",
			"x-x-4-5-x-x", []Finger{Pinky, Ring}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			if _, ok := place(held(t, tc.frets, tc.fingers...)); ok {
				t.Errorf("%s can be held, and it cannot", tc.what)
			}
		})
	}
}

// And the same notes held the way round a hand does are fine, which is what
// makes the test above about the crossing rather than about the notes.
func TestTheSameNotesHeldTheOtherWayRoundAreFine(t *testing.T) {
	for _, tc := range []struct {
		frets   string
		fingers []Finger
	}{
		{"x-x-2-3-x-x", []Finger{Index, Middle}},
		{"x-x-3-4-x-x", []Finger{Middle, Ring}},
		{"x-x-4-5-x-x", []Finger{Ring, Pinky}},
	} {
		if _, ok := place(held(t, tc.frets, tc.fingers...)); !ok {
			t.Errorf("%s cannot be held, and a hand does it every day", tc.frets)
		}
	}
}

// Where a crossing is not flatly impossible it still has to cost: three fingers
// laid across the strings in reverse is a hand turning itself inside out, and
// the fan they need to reach round is what says so.
func TestHoldingAShapeBackwardsCostsMore(t *testing.T) {
	forwards := held(t, "x-x-2-2-2-x", Index, Middle, Ring)
	backwards := held(t, "x-x-2-2-2-x", Ring, Middle, Index)
	a, ok := place(forwards)
	if !ok {
		t.Fatal("three fingers in a row on one fret is an open A")
	}
	b, ok := place(backwards)
	if ok && b <= a {
		t.Errorf("held the usual way it costs %.1f and backwards %.1f", a, b)
	}
}

// Both postures a hand rests in are postures the model can find, and neither is
// written down anywhere: one is the knuckle line along the neck and the other
// is the same line turned across the strings.
func TestBothRestingPosturesArePlaceable(t *testing.T) {
	alongTheNeck := held(t, "x-3-2-1-x-x", Ring, Middle, Index)
	acrossTheStrings := held(t, "x-x-2-2-2-x", Index, Middle, Ring)
	for _, v := range []Voicing{alongTheNeck, acrossTheStrings} {
		if _, ok := place(v); !ok {
			t.Errorf("%s cannot be held, and it is a shape a hand rests in", v.Grip())
		}
	}
}

// A finger left out gives slack rather than taking it, and nothing says so: the
// links between fingers add up in series, so two fingers with an idle one
// between them pull apart more easily than two side by side.
//
// This is the arithmetic rather than a shape, because a shape cannot show it on
// its own — swapping which fingers hold the notes changes their lengths too, and
// that would be measuring two things at once.
func TestAnIdleFingerGivesSlack(t *testing.T) {
	give := func(from, to int) float64 {
		total := 0.0
		for k := from; k < to; k++ {
			total += 1 / stiff[k]
		}
		return total
	}
	sideBySide := give(1, 2) // middle to ring
	oneApart := give(0, 2)   // index to ring, middle idle
	twoApart := give(0, 3)   // index to little, two idle
	if oneApart <= sideBySide {
		t.Errorf("with the middle finger idle a pair gives %.2f, side by side %.2f",
			oneApart, sideBySide)
	}
	if twoApart <= oneApart {
		t.Errorf("two idle fingers give %.2f and one gives %.2f", twoApart, oneApart)
	}
}

// Frets narrow as they climb, so the same shape is never harder further up the
// neck. This is the property the geometry exists to get right, and the model
// before it needed a term bolted on to fake it.
func TestAShapeIsNeverHarderFurtherUpTheNeck(t *testing.T) {
	for _, tc := range []struct{ low, high string }{
		{"1-3-3-2-1-1", "5-7-7-6-5-5"},
		{"x-1-3-3-3-1", "x-5-7-7-7-5"},
		{"x-3-2-1-x-x", "x-8-7-6-x-x"},
	} {
		lo, hi := Score(drawnFrets(t, tc.low)), Score(drawnFrets(t, tc.high))
		if hi.Strain() > lo.Strain() {
			t.Errorf("%s costs %d and the same shape at %s costs %d",
				tc.low, lo.Strain(), tc.high, hi.Strain())
		}
	}
}

func drawnFrets(t *testing.T, grip string) Voicing {
	t.Helper()
	f, ok := ParseGrip(grip)
	if !ok {
		t.Fatalf("%s: not a grip", grip)
	}
	return Voicing{Tuning: Standard(), Frets: f}
}

// One finger cannot hold two frets at once, barre or no barre.
func TestOneFingerCannotHoldTwoFrets(t *testing.T) {
	if _, ok := place(held(t, "x-x-3-4-x-x", Index, Index)); ok {
		t.Error("one finger held two different frets")
	}
}

// The thumb comes over the top of the neck, so it is not on the knuckle line
// and the geometry does not reach it. The shapes that use it are still shapes.
func TestTheThumbIsChargedButNotPlaced(t *testing.T) {
	with := held(t, "2-x-2-2-x-x", Thumb, Index, Middle)
	without := held(t, "x-x-2-2-x-x", Index, Middle)
	a, ok := place(with)
	if !ok {
		t.Fatal("a thumb over the top is a thing a hand does")
	}
	b, _ := place(without)
	if a <= b {
		t.Errorf("reaching over the neck with the thumb costs %.1f against %.1f "+
			"without, which cannot be right", a, b)
	}
}
