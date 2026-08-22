package voicing

import (
	"cmp"
	"slices"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
)

// Reading is one honest name for a set of notes.
type Reading struct {
	Chord    chord.Chord
	Quality  harmony.Quality
	Root     int   // pitch class
	Bass     int   // pitch class of the lowest note
	Omitted  []int // intervals of the quality that do not sound
	Inverted bool  // the lowest note is not the root
}

// preference breaks ties between readings of the same notes.
//
// Explicit rather than the order the Quality constants happen to be declared in:
// that order belongs to another package and could be rearranged by someone with
// no idea it was deciding what a fingering is called. Nearer the front means
// likelier to be what a player meant.
var preference = []harmony.Quality{
	harmony.Major, harmony.Minor,
	harmony.Dominant7, harmony.Minor7, harmony.Major7,
	harmony.Sus4, harmony.Sus2, harmony.Dominant7Sus4,
	harmony.Major6, harmony.Minor6,
	harmony.MinorMajor7, harmony.HalfDiminished,
	harmony.Diminished, harmony.Augmented, harmony.Diminished7,
	harmony.Add9, harmony.Dominant9, harmony.Major9, harmony.Minor9,
	harmony.Power,
}

var rank = func() map[harmony.Quality]int {
	m := make(map[harmony.Quality]int, len(preference))
	for i, q := range preference {
		m[q] = i
	}
	return m
}()

// minorish says which qualities are spelled the way a minor key is, so that a
// shape identified on pitch class 3 comes out Eb and not D#.
var minorish = map[harmony.Quality]bool{
	harmony.Minor: true, harmony.Minor7: true, harmony.MinorMajor7: true,
	harmony.Minor6: true, harmony.Minor9: true,
	harmony.Diminished: true, harmony.Diminished7: true, harmony.HalfDiminished: true,
}

// Identify names the notes a shape sounds, best reading first.
//
// A list and not a name, because the ambiguity is real and on a page for
// learning chords it is the interesting part: the same four notes are C6 or Am7
// depending on which is underneath, and a diminished seventh is four chords at
// once. An empty result means these notes are not a chord anyone has a name for,
// which is a better answer than inventing one.
func Identify(frets []Fret, t Tuning) []Reading {
	if t.IsZero() {
		t = Standard()
	}
	v := Voicing{Tuning: t, Frets: frets}
	bass, ok := v.Bass()
	if !ok {
		return nil
	}
	bassPC := bass % 12

	var out []Reading
	for root := range 12 {
		for _, q := range harmony.All() {
			r := Requirement{Root: root, Quality: q, Bass: -1}
			if ok, _ := r.Satisfies(v); !ok {
				continue
			}
			sounds := v.PitchClasses()
			var omitted []int
			for _, iv := range q.Intervals() {
				if !slices.Contains(sounds, (root+iv)%12) {
					omitted = append(omitted, iv)
				}
			}
			sp := chord.SpellingFor(root, minorish[q])
			c := chord.Chord{Root: chord.Spell(root, sp), Suffix: q.Symbol()}
			if bassPC != root {
				c.Bass, c.HasBass = chord.Spell(bassPC, sp), true
			}
			out = append(out, Reading{
				Chord: c, Quality: q, Root: root, Bass: bassPC,
				Omitted: omitted, Inverted: bassPC != root,
			})
		}
	}

	slices.SortFunc(out, func(a, b Reading) int {
		if d := cmp.Compare(len(a.Omitted), len(b.Omitted)); d != 0 {
			return d
		}
		if a.Inverted != b.Inverted {
			if a.Inverted {
				return 1
			}
			return -1
		}
		if d := cmp.Compare(rank[a.Quality], rank[b.Quality]); d != 0 {
			return d
		}
		return cmp.Compare(a.Root, b.Root)
	})
	return out
}
