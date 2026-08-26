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

// turns are the angles of the knuckle line that get tried, in degrees from the
// neck. Nought is a finger to a fret and ninety is the fingers in a row across
// the strings; a hand plays somewhere between and both ends are comfortable.
var turns = func() [12]struct{ deg, ux, uy, px, py float64 } {
	var t [12]struct{ deg, ux, uy, px, py float64 }
	for i := range t {
		deg := -10 + 10*float64(i)
		rad := deg * math.Pi / 180
		t[i].deg = deg
		t[i].ux, t[i].uy = math.Cos(rad), math.Sin(rad)
		t[i].px, t[i].py = -math.Sin(rad), math.Cos(rad)
	}
	return t
}()

// place is what the best hand placement for this shape costs, and whether there
// is one.
func place(v Voicing) (float64, bool) {
	tips, extra, ok := tipsOf(v)
	if !ok {
		return 0, false
	}
	if len(tips) == 0 {
		return extra, true // nothing fretted: open strings and a still hand
	}

	var lay laying
	lay.load(tips)

	best, at, found := math.Inf(1), -1, false
	for i := range turns {
		// Each angle starts the palm where the last one left it. Ten degrees
		// does not move the hand far, so the search begins near the answer and
		// most of the work disappears.
		if c, ok := lay.cost(i); ok && c < best {
			best, at, found = c, i, true
		}
	}
	if !found {
		return 0, false
	}

	// The winning angle, looked at closely. Where the fingers change the order
	// they have to lie in, the cost steps rather than curves, so it is worth
	// walking in from both sides rather than trusting a smooth minimum.
	for _, d := range [6]float64{-7, -4, -2, 2, 4, 7} {
		if c, ok := lay.costAt(turns[at].deg + d); ok && c < best {
			best = c
		}
	}
	return best + extra, true
}

// laying is one shape's fingertips, ready to be tried at angle after angle.
type laying struct {
	tx, ty [4]float64
	used   [4]bool
	n      int
	ax, ay float64 // where the palm ended up last time, and where to start
	warm   bool
}

func (l *laying) load(tips []tip) {
	*l = laying{}
	for _, t := range tips {
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
	return l.lay(deg, math.Cos(rad), math.Sin(rad), -math.Sin(rad), math.Cos(rad))
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

	if !l.warm {
		l.ax, l.ay = l.guess(ux, uy, px, py)
	}
	c, ok := l.settle(ux, uy, px, py, sinFree, sinMax)
	if !ok {
		// Nowhere from here. Try again from the shape's own guess before
		// giving up, in case the last angle left the palm somewhere useless.
		l.ax, l.ay = l.guess(ux, uy, px, py)
		if c, ok = l.settle(ux, uy, px, py, sinFree, sinMax); !ok {
			l.warm = false
			return 0, false
		}
	}
	l.warm = true
	return c + turn + spread, true
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

		// How far round from straight ahead the finger points, as the sine of
		// the angle. A hand fans without noticing, so there is a free band,
		// then a price, and then a limit.
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
	_ = px
	_ = py
	return total, true
}

// settle walks the palm one axis at a time, halving the step each round, until
// it stops finding anywhere better. It starts from wherever the last angle
// finished, which is usually within a millimetre or two.
func (l *laying) settle(ux, uy, px, py, sinFree, sinMax float64) (float64, bool) {
	best, ok := l.at(l.ax, l.ay, ux, uy, px, py, sinFree, sinMax)
	if !ok {
		l.ax, l.ay = l.guess(ux, uy, px, py)
		if best, ok = l.at(l.ax, l.ay, ux, uy, px, py, sinFree, sinMax); !ok {
			// Cast about a little before deciding the hand will not go here.
			for _, d := range [4][2]float64{{18, 0}, {-18, 0}, {0, 18}, {0, -18}} {
				if c, fits := l.at(l.ax+d[0], l.ay+d[1], ux, uy, px, py, sinFree, sinMax); fits {
					l.ax, l.ay, best, ok = l.ax+d[0], l.ay+d[1], c, true
					break
				}
			}
			if !ok {
				return 0, false
			}
		}
	}
	for step := 8.0; step >= 0.5; step /= 2 {
		for range 3 {
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
func tipsOf(v Voicing) ([]tip, float64, bool) {
	if len(v.Fingers) < len(v.Frets) {
		return nil, 0, false
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
			return nil, 0, false
		}
		extra += press(i, f)
		e := &at[k-Index]
		if !e.held {
			e.lo, e.hi, e.fret, e.held = i, i, f, true
			continue
		}
		if f != e.fret {
			return nil, 0, false // one finger cannot hold two frets
		}
		e.lo, e.hi = min(e.lo, i), max(e.hi, i)
	}

	var tips []tip
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
				return nil, 0, false
			}
			span := stringAt(e.hi, x) - stringAt(e.lo, x)
			extra += barreCost + barreSpanCost*span
			tips = append(tips, tip{finger: k, x: x,
				y: (stringAt(e.lo, x) + stringAt(e.hi, x)) / 2})
			continue
		}
		tips = append(tips, tip{finger: k, x: x, y: stringAt(e.lo, x)})
	}
	return tips, extra, true
}
