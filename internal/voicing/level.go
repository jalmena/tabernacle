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
// what it can produce is nothing anybody plays. The numbers are strain, not
// cost — what the hand is asked to do, with where it happens left out:
//
//	Em7 4, Em 8, A 12, E7 16    the two- and three-finger chords learnt first
//	E 20, Am 20, D 20, Dm 28,
//	C 28, G 30                  the rest of the open chords
//	Bm 49, F# 54, F 58          the barres everybody fights and then masters
//	B 64, Bb 68                 the A shape, a step past those: three
//	                            fingertips arching over a barre
//
// So trivial ends where the first group does, easy where the open chords do,
// moderate where the common barres do, and advanced where the A shape does.
// Those four are fixed by real chords: moving them would rename something
// everybody plays.
//
// The two above them are anchored to nothing, so they are set close enough
// together that the top band can actually be reached.
var levels = [6]int{8, 16, 35, 43, 51, 59}

// levelFor puts one strain into its band.
func levelFor(strain int) Level {
	for i, upto := range levels {
		if strain <= upto {
			return Level(i)
		}
	}
	return Inhuman
}

// Level says how hard this shape is to hold.
//
// It reads the strain rather than the cost. The cost carries a preference for
// shapes near the nut, which is how a chord reference orders its pages and not
// how a hand feels: frets narrow as they climb, so the same barre is easier at
// the ninth fret than at the first. Levelling by cost called one shape Media
// and the identical shape nine frets up Avanzada.
func (v Voicing) Level() Level {
	return levelFor(strain(v))
}

// holdable reports whether a fingering was found for this shape. Anything
// fretted without one needs more fingers than a hand has.
func holdable(v Voicing) bool { return v.Reach() == 0 || len(v.Fingers) > 0 }

// Score fills in Position and Cost for a shape somebody drew, so that it can be
// levelled like one the collection produced.
//
// A shape from the dataset arrives already scored. One drawn on the neck has
// only frets and fingers, and the rules that turn those into a number are this
// package's business and stay in it.
func Score(v Voicing) Voicing {
	if fret(&v) {
		v.Cost = cost(v)
		return v
	}

	// No fingering exists: the shape asks for more fingers than a hand has.
	// Without one, posture and spread have nothing to measure and would score
	// it as though it were comfortable, which had the impossible drawings
	// coming out cheaper than the merely difficult ones.
	//
	// No fingering exists. strain says what that is worth; the position and
	// fullness preferences still apply, because this is still a row in a list.
	v.Position = lowestFret(v.Frets)
	v.Cost = cost(v)
	return v
}

// shortBy is how many more fingers a shape needs than a hand has.
//
// A barre counts as one finger however many strings it covers, which is the
// only reason some shapes are holdable at all.
func shortBy(v Voicing) int {
	need := 0
	onBarre := 0
	b, barred := barreFor(v, 0)
	for i, f := range v.Frets {
		if f <= 0 {
			continue
		}
		if barred && i >= b.From && i <= b.To && int(f) == v.Position {
			onBarre++
			continue
		}
		need++
	}
	if onBarre > 0 {
		need++
	}
	return max(0, need-4)
}

// Strain is the number Level came from: what the shape asks of the hand, with
// where it sits on the neck and how full it is left out.
func (v Voicing) Strain() int { return strain(v) }
