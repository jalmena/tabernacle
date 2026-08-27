package voicing

// A hand laid on the neck, and what it costs to hold a shape with it.
//
// The hand is a straight line of knuckles and four fingers coming off it. Two
// numbers place it: how far the knuckle line is turned from the neck, and how
// far the hand sits from the strings. Everything a shape asks of it is measured
// against that — how far each finger reaches, how wide a pair opens, and how
// differently two fingers that share a tendon are asked to stretch.
//
// The point of doing it this way is that nothing here knows what a chord is.
// The model it replaces had twelve terms, each one added to save a shape that
// the eleven before it got wrong, and moving any of them moved three others. The
// terms below are lengths and stiffnesses. If one is wrong it is wrong in a way
// that can be checked against a hand.
//
// Which of them are measured and which are fitted is said at each one, and the
// tally is kept honest in docs/chord-diagrams.md. The hand they describe is a
// large one — a guitarist's — because it is the hand the judgements in
// testdata/judgements.json came from.

// Between the knuckles of two fingers side by side. This is the number the whole
// model turns on: it does not change with the fret, and the frets it is compared
// against run from 36.4 mm at the nut to 19.3 mm at the twelfth.
var knuckleGap = 21.0

// How far each finger reaches from its knuckle with the hand curled as it is for
// playing, index to little finger.
//
// Not the length of the finger. A hand on a neck holds the index flatter — it is
// the one that lies across for a barre — and the middle more arched, so what each
// one reaches in that posture is not the order a ruler gives. These are fitted
// like the costs, inside bounds a hand allows: the little finger reaches least,
// which is why the sixth string costs it something without anybody saying so.
var reach = [4]float64{85, 93, 88, 70}

// How hard it is to stretch two fingers by different amounts, index-middle,
// middle-ring, ring-little.
//
// These are the tendons, and they are why the three pairs are not
// interchangeable. A guitarist puts them at one, one and a half and nearly
// three, which is what they are. Fingers further apart in the hand get the
// three in series, so a pair with an idle finger between them comes out slacker
// than either link on its own — which is the slack a player feels when the ring
// finger has nothing to do.
var stiff = [3]float64{1.0, 1.5, 3.0}

// What using each finger costs before it is asked to do anything, index to
// little.
//
// This is the one thing in the file that is not geometry. The little finger is
// weak and the ring finger is not much better, and a player reaches for the
// middle finger over either of them even when the neck would rather they did
// not. Without it the model handed a three-note diagonal to the index, ring and
// little fingers — which lies flatter on a neck whose frets are wider than the
// knuckles are, and which nobody plays.
var weak = [4]float64{0, 0, 0.54, 8.87}

// What each kind of demand costs.
//
// These are the fitted ones, and they are variables rather than constants for a
// single reason: they were fitted by a program that varied them against the
// whole of testdata/judgements.json at once and kept what satisfied most of it.
// Fitting them one judgement at a time is what went wrong last time, and a
// number that can only be changed by editing the file it lives in is a number
// that gets changed one case at a time.
var (
	stretchCost = 0.0174 // per mm² reached past a finger's length
	curlCost    = 0.0017 // per mm² pulled up short of it
	freeFan     = 18.0   // degrees a finger points off straight without noticing
	fanCost     = 0.055  // per degree² past that
	spreadCost  = 0.0176 // per mm² a pair sits wider apart than its knuckles do
	squeezeCost = 0.0060 // per mm² it sits closer
	tendonCost  = 0.0028 // per mm² of difference between two fingers, times stiff
	wristCost   = 0.0037 // per degree² turned past what the wrist gives
	// A barre is the index laid flat: it stops being a fingertip and becomes a
	// bar, which is a different thing to ask and costs a flat amount plus the
	// length of neck it covers.
	barreCost     = 15.34
	barreSpanCost = 0.327 // per mm of string it lies across
	thumbCost     = 9.0   // reaching over the top of the neck, which is a different hand
	// Holding a note down cleanly. Two parts, and they are not the same kind of
	// number. The first is geometry: nearer the nut more string is left ringing,
	// so it swings wider for the same pluck and takes a firmer stop before it
	// rattles. That is real and it is small — five per cent between the first
	// fret and the second.
	//
	// A guitarist puts the first fret at half again the second, so the rest is
	// nutCost: a fitted number, not a derived one, and the only one in the file.
	// It stands for everything about the end of the neck that is not length —
	// the nut being a hard stop the string cannot give against, and the wrist
	// having to come further round to get there.
	pressCost = 3.54
	nutCost   = 45.17 // fitted against "half again as hard at the first fret"
	// The turn of the wrist, in degrees from the neck. Nought lays the knuckles
	// along the neck, a finger to a fret; ninety lays them across the strings,
	// the fingers in a row on one. A forearm rests between the two and turning
	// either way from there costs, gently.
	//
	// It has to cost something, or the angle is free — and a hand holding two
	// notes has three ways to move and only two of them spoken for, so it could
	// always turn until the shape fitted perfectly and every pair of notes came
	// out the same. Three notes apart on one string and one fret apart are not
	// the same, and this is what tells them apart.
	restTurn  = 53.0
	wristFrom = -12.0
	wristTo   = 100.0
)

// What a finger will not do at any price. These are the difference between a
// shape that is merely hard and one that is not a shape: past them the hand does
// not go, and the answer stops being a number and becomes a refusal.
var (
	maxFan   = 42.0  // degrees a finger can point off straight ahead
	maxReach = 1.10  // of its length, straightened out
	minReach = 0.595 // of its length, curled up as far as it goes
)

// damping is what muting a string in the middle of a shape costs.
var damping = 4.0

// gain turns what the placement costs into the numbers the levels are banded on.
var gain = 0.55

// A string is harder to stop the thicker it is. Sixth to first, relative.
var tension = [6]float64{1.30, 1.20, 1.12, 1.00, 0.94, 0.90}

// press is what it costs to hold one note down cleanly.
func press(s int, f Fret) float64 {
	if f <= 0 {
		return 0
	}
	ring := sounding(f) / sounding(12)
	near := nutCost / (fretAt(int(f)) + nutCost)
	return pressCost * tension[s] * ring * (1 + near)
}

// wrist is what turning the hand costs, and whether it turns that far at all.
func wrist(deg float64) (float64, bool) {
	if deg < wristFrom || deg > wristTo {
		return 0, false
	}
	d := deg - restTurn
	return wristCost * d * d, true
}

// tip is one finger with somewhere to be.
type tip struct {
	finger Finger
	x, y   float64 // where on the board the fingertip has to land
	extra  float64 // what this note costs to hold down, barre and all
}

// asymmetric charges a deviation, one rate for too much and another for too
// little. A hand gives more easily one way than the other in everything it does.
func asymmetric(d, over, under float64) float64 {
	if d > 0 {
		return over * d * d
	}
	return under * d * d
}
