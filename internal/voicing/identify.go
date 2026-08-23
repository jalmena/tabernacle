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

// worthFollowing reports whether stopping a string actually improves the reading.
//
// A reading with the root underneath always does: it turns an inversion into the
// chord itself, which is what somebody learning is reaching for. An inverted one
// only does if the strings being stopped were sounding a note the chord does not
// have — otherwise it is the same chord, thinner, over a different bass.
func worthFollowing(r Reading, drawn []int) bool {
	if !r.Inverted {
		return true
	}
	allowed := Requirement{Root: r.Root, Quality: r.Quality, Bass: -1}.Allowed()
	for _, pc := range drawn {
		if !slices.Contains(allowed, pc) {
			return true
		}
	}
	return false
}

// Suggestion is a chord a shape would play if some of its ringing strings were
// stopped.
type Suggestion struct {
	Reading
	// Silence is the strings to stop, lowest first.
	Silence []int
}

// maxSilence is how many ringing strings a suggestion may ask to be stopped.
//
// Two, because two is the number a beginner leaves ringing by accident: the pair
// of thick strings above a D or a Dm. Allowing three would start naming chords
// that have little to do with what is on the neck.
const maxSilence = 2

// Suggest names the chords a shape nearly plays, best first.
//
// Somebody learning does not reliably hear the difference between a string they
// have stopped and one they have left open, so a D drawn on an untouched neck
// comes out as three right notes over two wrong ones and gets no name at all.
// This is what turns that into "D, if you stop the sixth and the fifth" — which
// is both the answer and the lesson.
//
// Only open strings may be stopped. A fretted note was put there on purpose and
// is not a mistake to correct. A shape that would need a string silenced between
// two ringing ones is refused for the same reason the generator refuses to make
// one: nobody is going to damp it.
//
// And a suggestion has to be worth following. Stopping a string that belongs to
// the chord only thins it out — the plain D would otherwise be told it could be
// D/A, which corrects nothing. So a suggestion either takes away a note the
// chord does not have, or it puts the root underneath where it was not.
func Suggest(frets []Fret, t Tuning) []Suggestion {
	if t.IsZero() {
		t = Standard()
	}
	exact := make(map[string]bool)
	for _, r := range Identify(frets, t) {
		exact[r.Chord.String()] = true
	}

	drawn := Voicing{Tuning: t, Frets: frets}.PitchClasses()

	var open []int
	for i, f := range frets {
		if f == Open {
			open = append(open, i)
		}
	}
	if len(open) == 0 {
		return nil
	}

	best := make(map[string]Suggestion)
	for mask := 1; mask < 1<<len(open); mask++ {
		var pick []int
		for i, s := range open {
			if mask&(1<<i) != 0 {
				pick = append(pick, s)
			}
		}
		if len(pick) > maxSilence {
			continue
		}
		cand := slices.Clone(frets)
		for _, i := range pick {
			cand[i] = Muted
		}
		v := Voicing{Tuning: t, Frets: cand}
		if mutedInside(cand) > 0 || sounded(v) < 3 {
			continue
		}
		for _, r := range Identify(cand, t) {
			name := r.Chord.String()
			if exact[name] || !worthFollowing(r, drawn) {
				continue
			}
			// The cheapest way to reach a chord is the one worth suggesting.
			if had, ok := best[name]; ok && len(had.Silence) <= len(pick) {
				continue
			}
			best[name] = Suggestion{Reading: r, Silence: pick}
		}
	}

	out := make([]Suggestion, 0, len(best))
	for _, sg := range best {
		out = append(out, sg)
	}
	slices.SortFunc(out, func(a, b Suggestion) int {
		if d := cmp.Compare(len(a.Silence), len(b.Silence)); d != 0 {
			return d
		}
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
