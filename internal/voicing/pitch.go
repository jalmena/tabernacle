package voicing

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
)

// Pitches are the notes a shape sounds, as MIDI numbers, lowest string first.
// A muted string contributes nothing.
func (v Voicing) Pitches() []int {
	out := make([]int, 0, len(v.Frets))
	for i, f := range v.Frets {
		if f == Muted || i >= len(v.Tuning.Open) {
			continue
		}
		out = append(out, v.Tuning.Open[i]+int(f))
	}
	return out
}

// PitchClasses are the notes a shape sounds with the octaves forgotten, sorted
// and without repeats. This is what decides which chord a shape plays: doubling
// a note changes the sound and not the chord.
func (v Voicing) PitchClasses() []int {
	out := make([]int, 0, len(v.Frets))
	for _, p := range v.Pitches() {
		out = append(out, p%12)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Bass is the lowest note the shape sounds, as a MIDI number.
func (v Voicing) Bass() (int, bool) {
	ps := v.Pitches()
	if len(ps) == 0 {
		return 0, false
	}
	return slices.Min(ps), true
}

// Grip writes a shape the way a chord chart does: "x-3-2-0-1-0", lowest string
// first, x for a muted string.
//
// One string form, used for a URL and for a form value, so that a shape a reader
// drew and a shape the application drew are written the same way.
func (v Voicing) Grip() string {
	parts := make([]string, 0, len(v.Frets))
	for _, f := range v.Frets {
		if f == Muted {
			parts = append(parts, "x")
			continue
		}
		parts = append(parts, strconv.Itoa(int(f)))
	}
	return strings.Join(parts, "-")
}

// ParseGrip reads what Grip writes, reporting whether it was one.
//
// It is fed from a query string, so it refuses anything it does not recognise
// rather than guessing: a fret nobody has, a string count no guitar has, a word.
// It refuses surrounding whitespace too, so that one shape has one spelling and
// two links to the same grip are the same link.
func ParseGrip(s string) ([]Fret, bool) {
	parts := strings.Split(s, "-")
	if len(parts) < 4 || len(parts) > 8 {
		return nil, false
	}
	out := make([]Fret, 0, len(parts))
	for _, p := range parts {
		if p == "x" || p == "X" {
			out = append(out, Muted)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 24 {
			return nil, false
		}
		out = append(out, Fret(n))
	}
	return out, true
}

// Requirement is what a chord asks of a shape that claims to play it.
//
// The generator builds shapes against one, the identifier searches for the ones a
// shape satisfies, and the dataset's own test checks the hand-typed shapes with
// it. One rule, three readers, so none of them can drift from the others.
type Requirement struct {
	Root    int // pitch class
	Quality harmony.Quality
	Bass    int // pitch class, or -1 for no slash bass
}

// RequirementFor reads a chord symbol, reporting whether it is one this package
// models exactly. Decoration it does not model is a refusal, not a detail to
// round off: the b9 is the whole point of C7b9.
func RequirementFor(c chord.Chord) (Requirement, bool) {
	q, red, ok := harmony.Of(c)
	if !ok || !red.Exact() {
		return Requirement{}, false
	}
	req := Requirement{Root: c.Root.PitchClass(), Quality: q, Bass: -1}
	if c.HasBass {
		req.Bass = c.Bass.PitchClass()
	}
	return req, true
}

// Allowed is every pitch class that may sound. Anything else makes it a different
// chord.
func (r Requirement) Allowed() []int {
	out := make([]int, 0, 6)
	for _, iv := range r.Quality.Intervals() {
		out = append(out, (r.Root+iv)%12)
	}
	if r.Bass >= 0 {
		out = append(out, r.Bass)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Required is every pitch class that must sound.
//
// Everything the quality names, except a perfect fifth when the quality has four
// notes or more. That one sentence is the guitar literature's rule and it gives
// the right answer for every family here without a list of exceptions: a seventh
// chord may drop its fifth, a triad may not, and an *altered* fifth is never
// dropped because it is not seven semitones — the diminished's is six and the
// augmented's is eight, so the clause never reaches them.
//
// The four-note clause is also what stops two notes being called a chord: C and E
// alone are not C major.
func (r Requirement) Required() []int {
	tones := r.Quality.Intervals()
	out := make([]int, 0, len(tones)+1)
	for _, iv := range tones {
		if iv%12 == 7 && len(tones) >= 4 {
			continue // the fifth, which a seventh chord and up may leave out
		}
		out = append(out, (r.Root+iv)%12)
	}
	if r.Bass >= 0 {
		out = append(out, r.Bass)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Satisfies reports whether a shape plays this chord, and says why not when it
// does not.
//
// The reason is for tests and for the page that tells a reader what they have
// drawn; nothing branches on it.
func (r Requirement) Satisfies(v Voicing) (bool, string) {
	pitches := v.Pitches()
	if len(pitches) < 2 {
		return false, "fewer than two strings sound"
	}
	classes := v.PitchClasses()

	// The root first, so that the commonest refusal gets the clearest reason. A
	// rootless voicing is real practice, but it cannot honestly be captioned with
	// the chord's name on a page for learning chords.
	if !slices.Contains(classes, r.Root) {
		return false, "the root does not sound"
	}
	allowed := r.Allowed()
	for _, pc := range classes {
		if !slices.Contains(allowed, pc) {
			return false, "it sounds a note the chord does not have"
		}
	}
	for _, pc := range r.Required() {
		if !slices.Contains(classes, pc) {
			return false, "a note the chord needs is missing"
		}
	}

	lowest := slices.Min(pitches)
	if r.Bass >= 0 && lowest%12 != r.Bass {
		return false, "the named bass is not the lowest note"
	}

	// A ninth is a ninth because it is above the root, not merely because that
	// pitch class is present: a D below the E is a sus2 colour, not an added
	// ninth. Only these four families reach here, and they are exactly the ones
	// the curated dataset cannot cover.
	lowestRoot := 0
	for _, p := range pitches {
		if p%12 == r.Root {
			lowestRoot = p
			break
		}
	}
	for _, iv := range r.Quality.Intervals() {
		if iv <= 12 {
			continue
		}
		found := false
		for _, p := range pitches {
			if p%12 == (r.Root+iv)%12 && p > lowestRoot {
				found = true
				break
			}
		}
		if !found {
			return false, "the added note is not above the root"
		}
	}
	return true, ""
}
