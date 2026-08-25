package voicing

// Level is how hard a shape is to hold, on a scale a player recognises.
type Level int

// The levels, easiest first.
const (
	Trivial Level = iota
	Easy
	Moderate
	Advanced
	Complex
	Extreme
	Inhuman
)

// Key names the level for the message catalogue.
func (l Level) Key() string {
	switch l {
	case Trivial:
		return "level.trivial"
	case Easy:
		return "level.easy"
	case Moderate:
		return "level.moderate"
	case Advanced:
		return "level.advanced"
	case Complex:
		return "level.complex"
	case Extreme:
		return "level.extreme"
	}
	return "level.inhuman"
}

// levels are the upper bounds of each band, easiest first.
//
// They are anchored to chords a guitarist already has an opinion about rather
// than to quantiles of everything the generator can produce, because most of
// what it can produce is nothing anybody plays:
//
//	Em7 10, Em 14, E7 19          the two- and three-finger chords learnt first
//	E 23, A 24, G 36, C 37, D 38  the rest of the open chords
//	F 59, Bm 63                   the barres everybody fights and then masters
//	B 70                          the A-shape barre, a step past those
//
// So trivial ends where the first group does, easy where the open chords do,
// and moderate where the common barres do. Those three are fixed by real
// chords: moving them would rename something everybody plays.
//
// The three above them are not anchored to anything, so they are set close
// enough together that the top band can actually be reached. It used to begin
// at 145, past the ninety-ninth percentile of everything the generator can
// produce, which made it a label nobody would ever see.
var levels = [...]int{20, 40, 65, 78, 98, 120}

// Level says how hard this shape is to hold.
func (v Voicing) Level() Level {
	for i, upto := range levels {
		if v.Cost <= upto {
			return Level(i)
		}
	}
	return Inhuman
}

// Score fills in Position and Cost for a shape somebody drew, so that it can be
// levelled like one the collection produced.
//
// A shape from the dataset arrives already scored. One drawn on the neck has
// only frets and fingers, and the rules that turn those into a number are this
// package's business and stay in it.
func Score(v Voicing) Voicing {
	// Fingers first, and the shape may not be holdable at all: without them
	// posture has nothing to measure and every shape would look comfortable.
	if !fret(&v) {
		v.Position = lowestFret(v.Frets)
		v.Cost = cost(v)
		return v
	}
	v.Cost = cost(v)
	return v
}
