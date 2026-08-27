package voicing

import (
	"math"
	"testing"
)

// The frets fall where a string divides, and the numbers are checkable against
// a ruler on a real neck.
func TestTheFretsCrowdTogetherAsTheyClimb(t *testing.T) {
	want := []float64{36.4, 34.3, 32.4, 30.6, 28.9, 27.2, 25.7, 24.3, 22.9, 21.6, 20.4, 19.3}
	for n, w := range want {
		got := fretAt(n+1) - fretAt(n)
		if math.Abs(got-w) > 0.05 {
			t.Errorf("fret %d is %.1f mm wide, a 25.5\" neck makes it %.1f", n+1, got, w)
		}
	}
	if got := fretAt(12); math.Abs(got-scale/2) > 0.01 {
		t.Errorf("the twelfth fret is at %.1f mm and should halve the string at %.1f", got, scale/2)
	}
}

// The fact the whole model turns on: the frets are wider than a hand's knuckles
// low down and narrower high up, so one finger per fret is not a rule, it is what
// happens where those two numbers meet. Where exactly they meet depends on the
// hand — this one meets them at the fifteenth fret — and what matters is that
// they do meet, somewhere up the neck rather than at either end of it.
func TestTheFretsMeetTheKnucklesUpTheNeck(t *testing.T) {
	meets := 0
	for n := 1; n <= 20; n++ {
		if fretAt(n)-fretAt(n-1) <= knuckleGap {
			meets = n
			break
		}
	}
	switch {
	case meets == 0:
		t.Errorf("the frets never narrow to %.0f mm, so a hand never rests "+
			"one finger to a fret anywhere on the neck", knuckleGap)
	case meets < 6:
		t.Errorf("the frets narrow to a knuckle's width at fret %d, which would "+
			"make one finger per fret the shape of the hand nearly everywhere", meets)
	case meets > 17:
		t.Errorf("the frets do not narrow to a knuckle's width until fret %d, "+
			"which is off the end of where anybody plays chords", meets)
	}
}

// A fingertip lands in a stretch of board, not on a line, and the stretch is
// where the fret behind it ends and the fret itself begins.
func TestAFingertipLandsInACellAndAimsBehindTheFret(t *testing.T) {
	for f := Fret(1); f <= 12; f++ {
		lo, hi := cellOf(f)
		x := wants(f)
		if x <= lo || x > hi {
			t.Errorf("fret %d: a finger aims for %.1f, outside its cell %.1f–%.1f", f, x, lo, hi)
		}
	}
	if wants(1) >= wants(12) {
		t.Error("a fingertip aims further along the neck for a higher fret")
	}
}

// The neck widens toward the body and the strings widen with it.
func TestTheNeckWidensTowardTheBody(t *testing.T) {
	if spreadAt(0) != nutSpread {
		t.Errorf("at the nut the strings span %.1f, not %.1f", spreadAt(0), nutSpread)
	}
	if got := spreadAt(fretAt(12)); math.Abs(got-twelfthSpread) > 0.01 {
		t.Errorf("at the twelfth the strings span %.1f, not %.1f", got, twelfthSpread)
	}
	sixth, first := stringAt(0, fretAt(5)), stringAt(5, fretAt(5))
	if first <= sixth {
		t.Error("the first string sits on the far side of the neck from the sixth")
	}
	if mid := stringAt(0, fretAt(5)) + stringAt(5, fretAt(5)); math.Abs(mid) > 0.01 {
		t.Error("the strings are not centred on the neck")
	}
}

// Nearer the nut there is more string left ringing, so it swings wider and takes
// a firmer stop. A small effect, and the test says how small so nobody later
// mistakes it for the whole story.
func TestMoreStringRingsNearTheNut(t *testing.T) {
	if sounding(1) <= sounding(12) {
		t.Error("more string rings at the first fret than at the twelfth")
	}
	if got := sounding(1) / sounding(2); got > 1.10 {
		t.Errorf("the first fret rings %.2f times the second; that alone cannot "+
			"explain a barre being half again as hard", got)
	}
}
