package voicing

import "math"

// Placing the hand on the neck, which is the whole of the difficulty model.
//
// A shape does not have a difficulty. A hand holding it does, and the same shape
// can be held more than one way. So the question this file answers is: of every
// way this hand could be laid on the neck to reach these notes, what does the
// easiest one cost?
//
// That framing is what the term-by-term model could not do. Fingers crossing
// over, a finger pinned between two others, a hand asked to face two ways at
// once — none of them is a property of any pair of fingers, so no sum over pairs
// could see them. Here they are all the same thing: there is no way to lay the
// hand down.
//
// It has to be quick. The chord page works out four hundred shapes for a chord
// and every one of them is fingered several ways over, so the whole of this file
// gets something like five microseconds a placement. That is why the inner loop
// has no trigonometry in it, why the angles are a fixed table, and why each
// angle starts from where the last one finished.

// The angles of the knuckle line that get tried, in degrees from the
// neck.
const (
	turnFrom = -10.0
	turnTo   = 100.0
)

// turns are those angles on a five-degree grid, worked out once. Nought is a finger to a fret and ninety is the fingers in a row across
// the strings; a hand plays somewhere between and both ends are comfortable.
var turns = func() [23]struct{ deg, ux, uy, px, py float64 } {
	var t [23]struct{ deg, ux, uy, px, py float64 }
	for i := range t {
		deg := -10 + 5*float64(i)
		rad := deg * math.Pi / 180
		t[i].deg = deg
		t[i].ux, t[i].uy = math.Cos(rad), math.Sin(rad)
		t[i].px, t[i].py = math.Sin(rad), -math.Cos(rad)
	}
	return t
}()

// place is what the best hand placement for this shape costs, and whether there
// is one.
func place(v Voicing) (float64, bool) {
	tips, n, extra, ok := tipsOf(v)
	if !ok {
		return 0, false
	}
	if n == 0 {
		return extra, true // nothing fretted: open strings and a still hand
	}

	var lay laying
	lay.load(tips, n)

	// Each angle starts the palm where the last one left it, as well as from the
	// shape's own guess. Five degrees does not move the hand far, so most of the
	// searching is already done when a new angle begins.
	var coarse [len(turns)]float64
	best, found := math.Inf(1), false
	for i := range turns {
		coarse[i] = math.Inf(1)
		if c, ok := lay.cost(i); ok {
			coarse[i] = c
			if c < best {
				best, found = c, true
			}
		}
	}
	if !found {
		// Nothing on the grid. Before calling a shape unholdable — which is a
		// strong thing to say and the page says it in words — sweep finely,
		// because the angles at which a hand will go somewhere at all can be a
		// narrow band and five degrees can step over it. Rare, so the cost of
		// looking properly here is paid by almost nothing.
		for deg := turnFrom; deg <= turnTo; deg += 1 {
			if c, ok := lay.costAt(deg); ok && c < best {
				best, found = c, true
			}
		}
		if !found {
			return 0, false
		}
		return best + extra, true
	}

	// Then the angles worth a closer look, which is not only the best one. The
	// cost does not fall away smoothly: where the fingers change the order they
	// have to lie in it steps, so the surface has more than one valley and the
	// deepest is not always under the lowest coarse reading. Looking only under
	// that one came out a quarter over the answer on shapes with two ways to
	// hold them.
	margin := best * 1.35
	for i := range turns {
		if coarse[i] > margin {
			continue
		}
		for _, d := range [4]float64{-3.5, -1.75, 1.75, 3.5} {
			if c, ok := lay.costAt(turns[i].deg + d); ok && c < best {
				best = c
			}
		}
	}
	return best + extra, true
}

// The fingers come over the neck from the first string, not the sixth.
//
// A left hand goes under the neck with the thumb behind it and the fingers
// curling over the edge nearest the floor, which in playing position is the
// first string. So the palm is on that side and the sixth string is the one
// furthest from it — which is why reaching it with the little finger costs
// something, and why this was wrong for as long as it was: the model said that
// in words while its geometry had the hand on the other side of the neck.

// laying is one shape's fingertips, ready to be tried at angle after angle.
type laying struct {
	tx, ty [4]float64
	used   [4]bool
	n      int
	ax, ay float64 // where the palm ended up last time, and where to start
	warm   bool
}

func (l *laying) load(tips [4]tip, n int) {
	*l = laying{}
	for _, t := range tips[:n] {
		i := int(t.finger - Index)
		l.tx[i], l.ty[i], l.used[i] = t.x, t.y, true
		l.n++
	}
}

func (l *laying) cost(i int) (float64, bool) {
	t := turns[i]
	return l.lay(t.deg, t.ux, t.uy, t.px, t.py)
}

func (l *laying) costAt(deg float64) (float64, bool) {
	rad := deg * math.Pi / 180
	return l.lay(deg, math.Cos(rad), math.Sin(rad), math.Sin(rad), -math.Cos(rad))
}

// lay puts the knuckle line at one angle and works out what holding the shape
// costs there, or says the hand will not go.
func (l *laying) lay(deg, ux, uy, px, py float64) (float64, bool) {
	turn, ok := wrist(deg)
	if !ok {
		return 0, false
	}

	// The fingers lie in the order they grow in. A hand cannot put its middle
	// finger on the far side of its ring finger: that is not expensive, it is
	// not a thing. This one rule replaces the crossing, filling-order and
	// trapped-finger cases the old model had a term for each of.
	//
	// It also carries both resting postures. Turned along the neck the order
	// reads as one finger per fret; turned across the strings it reads as the
	// index on the graver string. Same rule, different angle.
	//
	// How far apart a pair sits along that line is charged here and not with the
	// rest, because it is the one thing the palm cannot fix. Move the hand and
	// every finger's reach and fan changes; the distance between two notes
	// measured along the knuckle line does not. Without it a shape held by two
	// fingers has three ways to move and only two of them spoken for, so the
	// hand could always turn until it fitted, and one fret apart came out the
	// same as four.
	//
	// Sitting closer together than the knuckles do is what curling does anyway.
	// Sitting wider is the work.
	spread := 0.0
	prev, last, first := 0.0, 0, true
	for i := range 4 {
		if !l.used[i] {
			continue
		}
		along := l.tx[i]*ux + l.ty[i]*uy
		if !first {
			if along < prev {
				return 0, false
			}
			spread += asymmetric(along-prev-knuckleGap*float64(i-last), spreadCost, squeezeCost)
		}
		prev, last, first = along, i, false
	}

	// Angles are turned into the sine of the angle once, here, so that the
	// inner loop can compare a ratio it already has instead of taking an arc
	// cosine four times for every step of the search.
	sinFree := math.Sin(freeFan * math.Pi / 180)
	sinMax := math.Sin(maxFan * math.Pi / 180)

	// Two starting places, and the better of them wins. Where the last angle
	// left the palm is usually within a millimetre or two of where this one
	// wants it, which is what makes the search quick; but not always, and a
	// search that walks downhill from a bad start stops at the first flat spot
	// it finds. Trying the shape's own guess as well caught placements coming
	// out half again over what they cost, and a handful of shapes being called
	// unholdable when a hand holds them.
	best, found := 0.0, false
	bx, by := 0.0, 0.0
	if l.warm {
		if c, ok := l.settle(ux, uy, px, py, sinFree, sinMax); ok {
			best, bx, by, found = c, l.ax, l.ay, true
		}
	}
	l.ax, l.ay = l.guess(ux, uy, px, py)
	if c, ok := l.settle(ux, uy, px, py, sinFree, sinMax); ok && (!found || c < best) {
		best, bx, by, found = c, l.ax, l.ay, true
	}
	if !found {
		l.warm = false
		return 0, false
	}
	l.ax, l.ay, l.warm = bx, by, true
	return best + turn + spread, true
}

// guess puts the palm where it would be if every finger lay straight out from
// the middle of the notes. It is close enough that the search rarely has far
// to go.
func (l *laying) guess(ux, uy, px, py float64) (float64, float64) {
	cx, cy, span := 0.0, 0.0, 0.0
	for i := range 4 {
		if !l.used[i] {
			continue
		}
		cx += l.tx[i] - float64(i)*knuckleGap*ux
		cy += l.ty[i] - float64(i)*knuckleGap*uy
		span += reach[i]
	}
	n := float64(l.n)
	return cx/n - span/n*px, cy/n - span/n*py
}

// at is what the hand pays with its palm at one place. The knuckles sit along
// the line from there; each finger is whatever runs from its own knuckle to the
// note, so reaching sideways lengthens it, exactly as a finger does.
//
// A placement that asks a finger for more than it has is refused, not charged.
// That is the whole of what inhuman means here.
func (l *laying) at(ax, ay, ux, uy, px, py, sinFree, sinMax float64) (float64, bool) {
	var ext [4]float64
	total := 0.0
	for i := range 4 {
		if !l.used[i] {
			continue
		}
		dx := l.tx[i] - (ax + float64(i)*knuckleGap*ux)
		dy := l.ty[i] - (ay + float64(i)*knuckleGap*uy)
		length := math.Sqrt(dx*dx + dy*dy)
		r := reach[i]
		if length > maxReach*r || length < minReach*r {
			return 0, false
		}
		ext[i] = length - r
		total += weak[i] + asymmetric(ext[i], stretchCost, curlCost)

		// A finger points away from its knuckle, over the strings. It does not
		// point back into the palm, and measuring only how far round it had
		// turned let it: with the angle taken from the knuckle line, a finger
		// aimed a hundred and eighty degrees the wrong way reads as perfectly
		// straight. A first fret and a fifth on the strings beside it came out
		// easy, held with an index pointing at the nut and a middle finger
		// pointing at the body.
		if dx*px+dy*py <= 0 {
			return 0, false
		}

		// How far round from straight ahead it points, as the sine of the
		// angle. A hand fans without noticing, so there is a free band, then a
		// price, and then a limit.
		lat := math.Abs(dx*ux+dy*uy) / length
		if lat > sinMax {
			return 0, false
		}
		if lat > sinFree {
			total += fanCost * (lat - sinFree) * (lat - sinFree)
		}
	}

	// Two fingers pulled to different lengths fight each other through the
	// tendon they share. Fingers with an idle one between them get the links in
	// series, which is exactly the slack a hand feels when the ring finger has
	// nothing to do — and it falls out rather than being written down.
	prev, first := 0, true
	for i := range 4 {
		if !l.used[i] {
			continue
		}
		if !first {
			give := 0.0
			for k := prev; k < i; k++ {
				give += 1 / stiff[k]
			}
			d := ext[i] - ext[prev]
			total += tendonCost * d * d / give
		}
		prev, first = i, false
	}
	return total, true
}

// settle walks the palm one axis at a time, halving the step each round, until
// it stops finding anywhere better. It starts from wherever the last angle
// finished, which is usually within a millimetre or two.
func (l *laying) settle(ux, uy, px, py, sinFree, sinMax float64) (float64, bool) {
	best, ok := l.at(l.ax, l.ay, ux, uy, px, py, sinFree, sinMax)
	if !ok {
		// The guess is the average of where the fingers want the palm, and an
		// average of somewhere and somewhere else can be nowhere: with three
		// fingers spread down the neck it fell outside what any of them could
		// reach, and the shape came out unholdable while the same shape with a
		// fourth finger added placed easily — which cannot be, since taking a
		// finger away only ever loosens a hand.
		//
		// So ask each finger in turn where the palm would be if it were the one
		// that had to stretch furthest, which is where a hand actually ends up,
		// and only then cast about.
		for i := range 4 {
			if !l.used[i] {
				continue
			}
			ax := l.tx[i] - float64(i)*knuckleGap*ux - reach[i]*px
			ay := l.ty[i] - float64(i)*knuckleGap*uy - reach[i]*py
			if c, fits := l.at(ax, ay, ux, uy, px, py, sinFree, sinMax); fits {
				l.ax, l.ay, best, ok = ax, ay, c, true
				break
			}
		}
	}
	if !ok {
		// Cast about before deciding the hand will not go here.
		for _, d := range [12][2]float64{
			{10, 0}, {-10, 0}, {0, 10}, {0, -10},
			{24, 0}, {-24, 0}, {0, 24}, {0, -24},
			{18, 18}, {18, -18}, {-18, 18}, {-18, -18},
		} {
			if c, fits := l.at(l.ax+d[0], l.ay+d[1], ux, uy, px, py, sinFree, sinMax); fits {
				l.ax, l.ay, best, ok = l.ax+d[0], l.ay+d[1], c, true
				break
			}
		}
		if !ok {
			return 0, false
		}
	}
	for step := 16.0; step >= 0.25; step /= 2 {
		for range 24 {
			moved := false
			for _, d := range [4][2]float64{{step, 0}, {-step, 0}, {0, step}, {0, -step}} {
				if c, fits := l.at(l.ax+d[0], l.ay+d[1], ux, uy, px, py, sinFree, sinMax); fits && c < best {
					l.ax, l.ay, best, moved = l.ax+d[0], l.ay+d[1], c, true
				}
			}
			if !moved {
				break
			}
		}
	}
	return best, true
}

// tipsOf turns a shape into somewhere for each finger to be, and adds up what
// the notes cost to hold down.
//
// A barre is one finger, so it gets one place to be — the middle of what it
// lies across — and pays for the whole length of it. Every string under it still
// has to be pressed.
func tipsOf(v Voicing) ([4]tip, int, float64, bool) {
	var tips [4]tip
	if len(v.Fingers) < len(v.Frets) {
		return tips, 0, 0, false
	}

	var barre *Barre
	if len(v.Barres) > 0 {
		barre = &v.Barres[0]
	}

	extra := 0.0
	var at [5]struct {
		lo, hi int
		fret   Fret
		held   bool
	}
	for i, f := range v.Frets {
		if f <= 0 {
			continue
		}
		k := v.Fingers[i]
		if k < Index || k > Thumb {
			return tips, 0, 0, false
		}
		extra += press(i, f)
		e := &at[k-Index]
		if !e.held {
			e.lo, e.hi, e.fret, e.held = i, i, f, true
			continue
		}
		if f != e.fret {
			return tips, 0, 0, false // one finger cannot hold two frets
		}
		e.lo, e.hi = min(e.lo, i), max(e.hi, i)
	}

	n := 0
	for k := Index; k <= Thumb; k++ {
		e := at[k-Index]
		if !e.held {
			continue
		}
		if k == Thumb {
			// The thumb comes over the top of the neck from the other side, so
			// it is not on the knuckle line and none of the geometry reaches
			// it. A few written-down shapes really do use it; they are charged
			// a flat amount and left out of the placement.
			extra += thumbCost
			continue
		}
		x := wants(e.fret)
		if e.hi > e.lo {
			// Laid flat across several strings.
			if k == Pinky || barre == nil || barre.Finger != k {
				return tips, 0, 0, false
			}
			span := stringAt(e.hi, x) - stringAt(e.lo, x)
			extra += barreCost + barreSpanCost*span
			tips[n] = tip{finger: k, x: x,
				y: (stringAt(e.lo, x) + stringAt(e.hi, x)) / 2}
			n++
			continue
		}
		tips[n] = tip{finger: k, x: x, y: stringAt(e.lo, x)}
		n++
	}
	return tips, n, extra, true
}

// Held is where the hand ends up: the placement the model settled on, in the
// millimetres the neck is measured in.
//
// It exists so the page can draw the hand it worked the difficulty out with. A
// model nobody can see is a model nobody can argue with, and every mistake in
// this one so far was found by somebody looking at a shape and saying that is
// not how you hold it.
type Held struct {
	// Turn is the angle of the knuckle line, in degrees from the neck: nought
	// lays the fingers one to a fret, ninety lays them in a row across.
	Turn float64
	// Palm is the first knuckle, and the rest follow it along the line.
	PalmX, PalmY float64
	Fingers      [4]Fingertip
}

// Fingertip is one finger of a placement, with where its knuckle sits and where
// its tip has to land.
type Fingertip struct {
	Used               bool
	KnuckleX, KnuckleY float64
	TipX, TipY         float64
	Stretch            float64 // millimetres past what the finger reaches, negative when curled
}

// Hold works out the best placement for a shape and says where the hand went.
//
// It repeats what place does rather than having place carry the answer around,
// because place runs several thousand times for a chord and once for a picture.
func Hold(v Voicing) (Held, bool) {
	tips, n, _, ok := tipsOf(v)
	if !ok || n == 0 {
		return Held{}, false
	}
	var lay laying
	lay.load(tips, n)

	best, at, found := math.Inf(1), 0.0, false
	for deg := turnFrom; deg <= turnTo; deg += 0.5 {
		var one laying
		one.load(tips, n)
		if c, ok := one.costAt(deg); ok && c < best {
			best, at, found = c, deg, true
		}
	}
	if !found {
		return Held{}, false
	}

	// Once more at the angle that won, to leave the palm where it settled.
	lay.warm = false
	if _, ok := lay.costAt(at); !ok {
		return Held{}, false
	}

	rad := at * math.Pi / 180
	ux, uy := math.Cos(rad), math.Sin(rad)
	h := Held{Turn: at, PalmX: lay.ax, PalmY: lay.ay}
	for i := range 4 {
		if !lay.used[i] {
			continue
		}
		kx := lay.ax + float64(i)*knuckleGap*ux
		ky := lay.ay + float64(i)*knuckleGap*uy
		h.Fingers[i] = Fingertip{
			Used:     true,
			KnuckleX: kx, KnuckleY: ky,
			TipX: lay.tx[i], TipY: lay.ty[i],
			Stretch: math.Sqrt((lay.tx[i]-kx)*(lay.tx[i]-kx)+(lay.ty[i]-ky)*(lay.ty[i]-ky)) - reach[i],
		}
	}
	return h, true
}

// Where converts a point on the neck into the coordinates a chord grid is drawn
// in: how far along the strings, sixth at nought and first at five, and how far
// down the frets, counted from the nut.
//
// Both come back fractional, because a knuckle does not sit on a fret line any
// more than it sits on a string.
func Where(x, y float64) (str, fret float64) {
	if x >= scale {
		x = scale - 0.01
	}
	if x > 0 {
		fret = -12 * math.Log2(1-x/scale)
	}
	w := spreadAt(math.Max(0, x))
	return (y + w/2) * 5 / w, fret
}
