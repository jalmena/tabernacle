package voicing

import (
	"cmp"
	"slices"

	"github.com/jalmena/tabernacle/internal/chord"
)

// A capo does one thing: it moves the nut. Everything above it is a guitar
// tuned higher, so the same shapes sound higher notes, and a chord that needed
// a barre can be reached with the shape of an easier one.
//
// The arithmetic is short. What you hear is what you finger plus where the capo
// sits, so with the sound held still, moving the capo from A to B means
// fingering every chord (B − A) semitones lower. The whole of this file is that
// sentence, applied to each fret in turn and scored.

// maxCapo is as far up as an option is offered. Past the seventh fret there is
// not much guitar left above the capo and the tone thins towards a mandolin.
const maxCapo = 7

// CapoOption is one place to put the capo and what the song becomes there.
type CapoOption struct {
	Fret    int           // where the capo goes; 0 is none
	Key     chord.Chord   // the key the left hand is playing in
	Chords  []chord.Chord // what the left hand actually holds
	Open    int           // how many of those are open-position shapes
	Barred  int           // how many need a barre
	Unknown int           // how many we hold no shape for
	Score   int           // lower is easier; see difficulty
}

// difficulty scores one shape the way a player ranks it.
//
// The numbers matter only against each other. A chord with no shape at all
// dominates everything, because no arrangement of the others rescues a song you
// cannot play; a barre is a real obstacle and worth several closed shapes; an
// open chord is free.
func difficulty(res Result) (score int, open, barred, unknown bool) {
	if !res.Found() {
		return 100, false, false, true
	}
	v := res.Voicings[0]
	switch {
	case v.HasBarre():
		return 10, false, true, false
	case v.HasOpenString() && v.Reach() <= 4:
		return 0, true, false, false
	default:
		return 3, false, false, false
	}
}

// CapoOptions ranks the places a capo could go for one song.
//
// key and chords are the sheet as it stands and at is where the capo sits
// today, so a song already written for a capo is measured from where it is
// rather than from the nut. The song's own key decides the spelling of every
// option, which is the rule song.Transpose already follows: a sheet moved into
// Eb reads in flats throughout instead of mixing A# and Bb.
//
// The result is easiest first and always contains every fret from none to
// maxCapo, so a caller can compare an option against doing nothing.
func CapoOptions(key chord.Chord, chords []chord.Chord, at int, opts Options) []CapoOption {
	// The same chord twice is one problem, not two: a song with nine G's and
	// one F is an easy song with one hard chord in it.
	var unique []chord.Chord
	for _, c := range chords {
		if !slices.ContainsFunc(unique, func(o chord.Chord) bool { return o.String() == c.String() }) {
			unique = append(unique, c)
		}
	}
	if len(unique) == 0 {
		return nil
	}

	var out []CapoOption
	for fret := 0; fret <= maxCapo; fret++ {
		// Held sound, moved nut: finger everything (fret − at) lower.
		by := at - fret
		newKey, spelling := chord.TransposeKey(key, by)

		opt := CapoOption{Fret: fret, Key: newKey}
		for _, c := range unique {
			moved := c
			if by != 0 {
				moved = c.Transpose(by, spelling)
			}
			opt.Chords = append(opt.Chords, moved)

			score, open, barred, unknown := difficulty(For(moved, opts))
			opt.Score += score
			if open {
				opt.Open++
			}
			if barred {
				opt.Barred++
			}
			if unknown {
				opt.Unknown++
			}
		}
		// A capo is a thing you have to own, fit and remember to take off, so it
		// has to earn its place: this nudge only ever decides a near tie.
		opt.Score += fret
		out = append(out, opt)
	}

	slices.SortStableFunc(out, func(a, b CapoOption) int { return cmp.Compare(a.Score, b.Score) })
	return out
}
