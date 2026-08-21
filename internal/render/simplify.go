package render

import (
	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// maxSimplifySteps bounds the walk down a chord's family tree. Four is more than
// any chain in the table needs; the bound is there so a future family with a
// mistake in its parent cannot spin.
const maxSimplifySteps = 4

// simplifier rewrites chords into ones the collection can actually play.
//
// It stops at the first form that has a real shape rather than walking all the
// way to a triad. Cmaj7#11 becomes Cmaj7, which is playable and still sounds
// like the chord that was written; carrying on to a bare C would throw away
// character for nothing.
//
// A chord that cannot be brought anywhere playable is left exactly as it was.
// Rewriting it would cost the reader the real chord and give nothing back.
type simplifier struct {
	opts  voicing.Options
	cache map[string]chord.Chord
	moved int
}

func newSimplifier(opts voicing.Options) *simplifier {
	return &simplifier{opts: opts, cache: make(map[string]chord.Chord)}
}

// chord returns the playable stand-in for c, or c itself.
func (sp *simplifier) chord(c chord.Chord) chord.Chord {
	key := c.String()
	if got, ok := sp.cache[key]; ok {
		if got.String() != key {
			sp.moved++
		}
		return got
	}

	out := c
	for _, cand := range candidates(c) {
		if sp.playable(cand) {
			out = cand
			break
		}
	}
	sp.cache[key] = out
	if out.String() != key {
		sp.moved++
	}
	return out
}

// candidates lists what to try for a chord, most faithful first: the chord
// itself, then without its slash bass, then each simpler family with and
// without it.
//
// The bass goes before the quality because on a guitar sheet it is the least of
// the chord: an inversion is usually somebody else's note, while the quality is
// what the chord is. Cmaj7/E is better served by Cmaj7 than by C/E.
//
// The walk is bounded rather than run to exhaustion so that a future family with
// a mistake in its parent cannot spin here.
func candidates(c chord.Chord) []chord.Chord {
	dropBass := func(x chord.Chord) chord.Chord {
		x.HasBass = false
		return x
	}
	out := []chord.Chord{c}
	if c.HasBass {
		out = append(out, dropBass(c))
	}
	cur := c
	for range maxSimplifySteps {
		next, changed := harmony.Simplify(cur)
		if !changed {
			break
		}
		cur = next
		out = append(out, cur)
		if cur.HasBass {
			out = append(out, dropBass(cur))
		}
	}
	return out
}

// playable reports whether the collection holds a shape for this chord. Since
// voicing.For never substitutes, having a shape at all means having a real one.
func (sp *simplifier) playable(c chord.Chord) bool {
	return voicing.For(c, sp.opts).Found()
}

// Moved is how many chord occurrences were rewritten.
func (sp *simplifier) Moved() int { return sp.moved }

// simplify rewrites a song's chords and reports how many changed.
func simplify(s song.Song, opts voicing.Options) (song.Song, int) {
	sp := newSimplifier(opts)
	out := s.MapChords(sp.chord)
	return out, sp.Moved()
}

// wouldSimplify reports whether offering to simplify would change anything.
//
// The control is hidden otherwise. A button that does nothing when pressed is
// worse than an absent one, and most songs have nothing to simplify.
func wouldSimplify(s song.Song, opts voicing.Options) bool {
	sp := newSimplifier(opts)
	for _, c := range s.Chords() {
		if sp.chord(c).String() != c.String() {
			return true
		}
	}
	return false
}
