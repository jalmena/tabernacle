package voicing

import (
	"cmp"
	"math"
	"slices"

	"github.com/jalmena/tabernacle/internal/chord"
)

// GenerateOptions tunes the search. The zero value is standard tuning over the
// first twelve frets, which is what the page wants.
type GenerateOptions struct {
	Tuning Tuning
	// MaxPosition is the highest fret the hand may start on. Zero means twelve,
	// where the octave repeats and everything above is a shape already shown.
	MaxPosition int
}

func (o GenerateOptions) tuning() Tuning {
	if o.Tuning.IsZero() {
		return Standard()
	}
	return o.Tuning
}

func (o GenerateOptions) maxPosition() int {
	if o.MaxPosition <= 0 {
		return maxPosition
	}
	return o.MaxPosition
}

// shapesFor finds every playable way to hold a chord, easiest first.
//
// The search walks a four-fret window up the neck. Four, not the five a diagram
// draws: maxSpan is 4, so every shape a hand can hold fits inside four
// consecutive frets, and a wider window would only rediscover the same shapes.
// Drawing five frets is a matter of the picture breathing; reaching four is a
// matter of the hand.
func shapesFor(r Requirement, opts GenerateOptions) []Voicing {
	t := opts.tuning()
	n := t.Strings()
	if n == 0 {
		return nil
	}
	allowed := 0
	for _, pc := range r.Allowed() {
		allowed |= 1 << pc
	}
	needed := len(r.Required())

	seen := make(map[string]bool)
	var out []Voicing
	frets := make([]Fret, n)

	for start := 1; start <= opts.maxPosition(); start++ {
		// Per string, the frets worth trying: silence, the open string when its
		// note belongs to the chord, and the frets of this window that do.
		choices := make([][]Fret, n)
		for s := range n {
			c := []Fret{Muted}
			fits := func(f int) bool { return allowed&(1<<((t.Open[s]+f)%12)) != 0 }
			if fits(0) {
				c = append(c, Open)
			}
			for f := start; f < start+maxSpan; f++ {
				if fits(f) {
					c = append(c, Fret(f))
				}
			}
			choices[s] = c
		}

		var walk func(s, classes, sounding int)
		walk = func(s, classes, sounding int) {
			if s == n {
				if sounding < 2 || popcount(classes) < needed {
					return
				}
				v := Voicing{Tuning: t, Frets: slices.Clone(frets)}
				// A silent string between two ringing ones has to be damped
				// deliberately, with a fingertip leaning off a note that is
				// already being held. It is a real technique and not a shape to
				// hand somebody learning a chord, and no one of the fifty
				// hand-written shapes does it.
				if v.Span() > maxSpan || mutedInside(v.Frets) > 0 {
					return
				}
				if ok, _ := r.Satisfies(v); !ok {
					return
				}
				k := fretKey(v.Frets)
				if seen[k] {
					return
				}
				if !fret(&v) || v.Position > opts.maxPosition() {
					return
				}
				seen[k] = true
				out = append(out, v)
				return
			}
			for _, f := range choices[s] {
				frets[s] = f
				if f == Muted {
					walk(s+1, classes, sounding)
					continue
				}
				walk(s+1, classes|1<<((t.Open[s]+int(f))%12), sounding+1)
			}
			frets[s] = Muted
		}
		walk(0, 0, 0)
	}

	written := adoptDataset(r, opts, out)
	full := fullness(r)

	// The order a chord reference wants, which is not the order a song sheet
	// wants.
	//
	// A shape somebody wrote down for this exact chord comes first, easiest of
	// those first: the dataset saying "C is x 3 2 0 1 0" settles the question,
	// and there is nothing to weigh it against. Everything else — a movable
	// template slid up to this root as much as a shape the search found — is a
	// derivation, and derivations compete: fullest, then easiest.
	//
	// Fullness before cost is deliberate down there, and it stops counting once
	// the chord is whole. Cost measures how hard a grip is to hold, and by that
	// measure the thinnest shape always wins: for Csus4 it would put two notes
	// on two strings ahead of the voicing everyone is taught. Past the point
	// where every note sounds and one is doubled, another string adds nothing a
	// reader is looking for, and rewarding it would hand first place to a barre
	// at the eighth fret over an open shape a beginner could hold today.
	slices.SortFunc(out, func(a, b Voicing) int {
		ka, kb := fretKey(a.Frets), fretKey(b.Frets)
		if written[ka] != written[kb] {
			if written[ka] {
				return -1
			}
			return 1
		}
		if written[ka] {
			if d := cmp.Compare(a.Cost, b.Cost); d != 0 {
				return d
			}
			return cmp.Compare(ka, kb)
		}
		for _, d := range []int{
			cmp.Compare(full(b), full(a)),
			cmp.Compare(a.Cost, b.Cost),
			cmp.Compare(ka, kb),
		} {
			if d != 0 {
				return d
			}
		}
		return 0
	})
	return out
}

// sounded counts the strings that make a note.
func sounded(v Voicing) int {
	n := 0
	for _, f := range v.Frets {
		if f != Muted {
			n++
		}
	}
	return n
}

// fullness measures how complete a voicing sounds, and stops counting once the
// chord is whole.
//
// Whole means every note of the chord plus one doubling, which is the point past
// which another string can only repeat something already sounding. It depends on
// the chord: a triad is whole on four strings, and a power chord — two notes —
// on three. Counting further would rank a six-string barre above the open shape
// a player actually wants; counting less would rank two notes on two strings
// above the whole chord.
func fullness(r Requirement) func(Voicing) int {
	whole := min(4, len(r.Quality.Intervals())+1)
	return func(v Voicing) int { return min(sounded(v), whole) }
}

func popcount(x int) int {
	n := 0
	for ; x != 0; x &= x - 1 {
		n++
	}
	return n
}

// fret works out how to hold a shape, reporting whether a hand can.
//
// Deterministic and without the thumb: a reference has to give the same answer
// every time, and a thumb-over grip is a preference, not a fingering everyone
// can use. Fingers go on in order of fret and then of string, which is what a
// player does anyway — it is why the ordinary D and Dm come out with the
// fingering they are taught with.
//
// Whether to lay a barre is decided by trying it. A barre is not only how a
// hand reaches six strings with four fingers; it is also how it frees a finger
// that is needed somewhere else, and a rule that counts fretted strings cannot
// see the second reason. It made 1-5-4-1-0-0 spend the middle finger on the
// third string at the first fret while the ring and little fingers were at the
// fourth and fifth — a fingering no player would use, and one that scored the
// shape harder than the same shape with a note added, because adding the note
// pushed it over the counting rule and let it barre.
//
// So both fingerings are worked out and the cheaper is kept, which is what the
// hand does. E major and A major still come out with three separate fingers, F
// and Bm with the full barre, and F/C with the small one across the top two
// strings — not because they are named here, but because that is what wins.
func fret(v *Voicing) bool {
	plain := *v
	plain.Frets = v.Frets
	withBarre := *v
	withBarre.Frets = v.Frets

	okPlain := fretWith(&plain, false)
	// Only work the barre out when there could be one. Without this the same
	// fingering gets found twice for most shapes, and finding it is the
	// expensive part.
	okBarre := couldBarre(*v) && fretWith(&withBarre, true)
	switch {
	case okPlain && okBarre:
		if withBarre.Cost < plain.Cost {
			*v = withBarre
		} else {
			*v = plain
		}
	case okPlain:
		*v = plain
	case okBarre:
		*v = withBarre
	default:
		return false
	}
	return true
}

// couldBarre reports whether a barre is even on the table for this shape.
func couldBarre(v Voicing) bool {
	lowest, fretted := 0, 0
	for _, f := range v.Frets {
		if f <= 0 {
			continue
		}
		fretted++
		if lowest == 0 || int(f) < lowest {
			lowest = int(f)
		}
	}
	if lowest == 0 {
		return false
	}
	v.Position = lowest
	_, ok := barreFor(v, fretted)
	return ok
}

// fretWith works out one fingering, with a barre or without.
func fretWith(v *Voicing, barre bool) bool {
	v.Position = 0
	for _, f := range v.Frets {
		if f > 0 && (v.Position == 0 || int(f) < v.Position) {
			v.Position = int(f)
		}
	}
	v.Fingers = make([]Finger, len(v.Frets))
	v.Barres = nil
	if v.Position == 0 {
		v.Cost = cost(*v)
		return true
	}

	assign := make([]int, 0, len(v.Frets)) // string indices still needing a finger
	for i, f := range v.Frets {
		if f > 0 {
			assign = append(assign, i)
		}
	}

	if b, ok := barreFor(*v, len(assign)); barre && ok {
		v.Barres = []Barre{b}
		rest := assign[:0:0]
		for _, i := range assign {
			if i >= b.From && i <= b.To && int(v.Frets[i]) == v.Position {
				v.Fingers[i] = b.Finger
				continue
			}
			rest = append(rest, i)
		}
		assign = rest
	}

	slices.SortFunc(assign, func(a, b int) int {
		if d := cmp.Compare(v.Frets[a], v.Frets[b]); d != 0 {
			return d
		}
		return cmp.Compare(a, b)
	})
	first := Index
	if v.HasBarre() {
		first = Middle
	}
	return chooseFingers(v, assign, first)
}

// chooseFingers hands the notes out to fingers, trying every choice of which
// fingers to use and keeping the one the hand settles on most easily.
//
// Fingers still go on in order of fret and then of string, because a hand cannot
// cross them over. What is chosen is which of the four are used at all, and
// there are never more than a handful of ways: six for two fingers, four for
// three, one for all of them.
//
// Taking them in turn from the index — which is what this replaces — meant two
// notes two frets apart came out as the index and the middle finger, which no
// hand does. Nothing here says which finger ought to go where. The knuckles are
// twenty-one millimetres apart and the frets are wider than that near the nut,
// so skipping a finger to skip a fret is simply what the hand settles on, and
// what a player calls one finger per fret is that settling written down.
func chooseFingers(v *Voicing, assign []int, first Finger) bool {
	if len(assign) == 0 {
		v.Cost = cost(*v)
		return true
	}
	if len(assign) > int(Pinky-first)+1 {
		return false
	}

	best, ok := math.Inf(1), false
	var chosen []Finger
	var pick func(at int, next Finger, used []Finger)
	pick = func(at int, next Finger, used []Finger) {
		if at == len(assign) {
			for j, i := range assign {
				v.Fingers[i] = used[j]
			}
			if c, fits := place(*v); fits && c < best {
				best, ok = c, true
				chosen = append(chosen[:0], used...)
			}
			return
		}
		for f := next; int(Pinky-f)+1 >= len(assign)-at; f++ {
			pick(at+1, f+1, append(used, f))
		}
	}
	pick(0, first, make([]Finger, 0, len(assign)))
	if ok {
		for j, i := range assign {
			v.Fingers[i] = chosen[j]
		}
		// The placement that won is the shape's difficulty. Working it out
		// again from scratch here doubled the cost of generating a chord.
		v.Cost = scored(*v, best) + 3*v.Position + 6*silent(v.Frets)
		return true
	}
	{
		// No choice of fingers lets the hand reach. Leave the first one on so
		// the shape can still be described, and let strain say what it is.
		next := first
		for _, i := range assign {
			if next > Pinky {
				return false
			}
			v.Fingers[i] = next
			next++
		}
		v.Cost = cost(*v)
		return true
	}
}

// barreFor says where the index finger would lie if it lay flat across the
// lowest fret of a shape. Whether it should is fret's question, not this one.
//
// It spans from the first sounding string at that fret to the last, and refuses
// if an open string falls inside: a finger lying across cannot let one ring.
func barreFor(v Voicing, fretted int) (Barre, bool) {
	lo, hi := -1, -1
	for i, f := range v.Frets {
		if int(f) == v.Position {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 || hi == lo {
		return Barre{}, false
	}
	for i := lo; i <= hi; i++ {
		if v.Frets[i] == Open {
			return Barre{}, false
		}
	}
	return Barre{Fret: Fret(v.Position), From: lo, To: hi, Finger: 1}, true
}

// adoptDataset replaces a generated shape with the hand-written one when they
// are the same grip, and reports which of them were written for this exact
// chord.
//
// The generator's fingerings are correct; the dataset's are idiomatic, and they
// carry the CAGED name that makes a shape recognisable. Where a person has
// already decided how a shape is held, that decision wins.
//
// The two kinds of entry are not equally authoritative and the set says which is
// which. A curated entry names a chord and a grip together: somebody decided
// that this is how this chord is held. A movable template is a shape slid until
// its root lands on the right note, which is arithmetic — useful, often the only
// answer high up the neck, but no reason to put a barre at the eighth fret ahead
// of an open shape a beginner could hold today.
func adoptDataset(r Requirement, opts GenerateOptions, out []Voicing) map[string]bool {
	ds, err := load()
	if err != nil {
		return nil
	}
	k := key{root: r.Root, quality: r.Quality, bass: r.Bass}
	known := gather(ds, k, opts.tuning())
	if len(known) == 0 {
		return nil
	}
	byGrip := make(map[string]Voicing, len(known))
	for _, v := range dedupe(known) {
		byGrip[fretKey(v.Frets)] = v
	}
	written := make(map[string]bool, len(ds.curated[k])+1)
	for _, v := range ds.curated[k] {
		written[fretKey(v.Frets)] = true
	}
	// A chord with nothing curated still has a shape the collection prints:
	// the first one a lookup returns, which for every root that needs a barre
	// is that barre at the position a reference shows it. Without this the page
	// leads with whatever the generator finds cheapest — for B, a four-string
	// grip with the top string muted — and then disagrees with the diagram the
	// same chord gets on a song sheet.
	//
	// Only the first, and only when nothing is curated. Adopting every slid
	// template put a barre at the eighth fret above the open C.
	if len(ds.curated[k]) == 0 {
		if best := dedupe(known); len(best) > 0 {
			slices.SortStableFunc(best, func(a, b Voicing) int { return cmp.Compare(a.Cost, b.Cost) })
			written[fretKey(best[0].Frets)] = true
		}
	}
	for i, v := range out {
		if c, ok := byGrip[fretKey(v.Frets)]; ok {
			out[i] = c
		}
	}
	return written
}

// Group is every way of playing one written chord — the chord itself, or one of
// its inversions, which is a different symbol and gets its own.
//
// Sorted easiest first. There is no trimming here: how many diagrams a page can
// hold is a question about the page, and this package has never seen one.
type Group struct {
	Chord  chord.Chord
	Shapes []Voicing
}

// Generated is the answer to "show me this chord", reporting whether the symbol
// is one this package can voice exactly.
//
// Root and Inversions never mix. A diagram captioned C whose lowest note is E is
// a lie by omission, and the chord already has a name for that: C/E.
type Generated struct {
	Chord      chord.Chord
	Root       Group
	Inversions []Group
}

// Generate finds every way to play a chord, separated by what is in the bass.
func Generate(c chord.Chord, opts GenerateOptions) (Generated, bool) {
	r, ok := RequirementFor(c)
	if !ok {
		return Generated{}, false
	}

	byBass := make(map[int][]Voicing)
	for _, v := range shapesFor(r, opts) {
		b, okBass := v.Bass()
		if !okBass {
			continue
		}
		byBass[b%12] = append(byBass[b%12], v)
	}

	// The chord's own group is the one with the right note underneath, which for
	// a slash chord is the bass it names rather than its root. C/E asks for E in
	// the bass; every shape that answers is C/E, and none of them is an
	// inversion of it.
	main := r.Root
	if r.Bass >= 0 {
		main = r.Bass
	}
	g := Generated{Chord: c, Root: Group{Chord: c, Shapes: byBass[main]}}
	delete(byBass, main)

	rest := make([]int, 0, len(byBass))
	for pc := range byBass {
		rest = append(rest, pc)
	}
	// Ordered by how far the bass sits above the root, so the third comes before
	// the fifth and the fifth before the seventh: the order inversions are
	// taught in.
	slices.SortFunc(rest, func(a, b int) int {
		return cmp.Compare(((a-r.Root)%12+12)%12, ((b-r.Root)%12+12)%12)
	})
	sp := chord.SpellingForKey(c)
	for _, pc := range rest {
		over := c
		over.Bass = chord.Spell(pc, sp)
		over.HasBass = true
		g.Inversions = append(g.Inversions, Group{Chord: over, Shapes: byBass[pc]})
	}
	return g, true
}
