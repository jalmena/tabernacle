// Package voicing resolves a chord to the shapes a guitarist actually plays.
//
// The shapes come from a curated dataset rather than from generating every
// fingering that satisfies the chord tones. Generation produces mostly
// unplayable nonsense, and the point of a chord diagram is to show something a
// hand can do.
//
// The dataset has two kinds of entry. Curated shapes are written out in full and
// are how the open-position fingerings everyone actually uses get in, since those
// are idiomatic rather than derived. Movable shapes are templates that slide up
// the neck, which is how a handful of entries cover all twelve roots.
package voicing

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/harmony"
)

// Fret is a position on one string, counted from the nut.
type Fret int8

const (
	Muted Fret = -1 // the string is not sounded
	Open  Fret = 0  // the string is sounded unfretted
)

// Finger identifies which finger frets a note.
type Finger int8

const (
	NoFinger Finger = 0
	Index    Finger = 1
	Middle   Finger = 2
	Ring     Finger = 3
	Pinky    Finger = 4
	// Thumb exists because a few idiomatic shapes really do use it, and
	// pretending otherwise would misdescribe them.
	Thumb Finger = 5
)

// Barre is one finger holding several strings at one fret.
//
// Barres are recorded in the dataset, never inferred from the fret numbers. E
// major is 0 2 2 1 0 0: two strings at the second fret, played with two separate
// fingers. Inferring a barre there would draw a shape nobody plays.
type Barre struct {
	Fret     Fret
	From, To int // string indices, inclusive, lowest string first
	Finger   Finger
}

// Voicing is one playable shape.
//
// Frets and Fingers are indexed lowest string first, the order chord grids are
// written in, and Frets are absolute so that a movable shape and its
// open-position cousin stay directly comparable.
type Voicing struct {
	Tuning   Tuning
	Frets    []Fret
	Fingers  []Finger
	Barres   []Barre
	Position int    // lowest fretted fret; 0 for a shape with none
	Shape    string // "open", "e-shape", "a-shape", "d-shape"
	Approx   Approximation
	Cost     int // playability score, and the sort key
}

// Strings is the number of strings the shape covers. It is a method rather than
// a field because a field is an invariant that can go stale.
func (v Voicing) Strings() int { return len(v.Frets) }

// Span is how many frets the hand must cover.
func (v Voicing) Span() int {
	lo, hi := 0, 0
	for _, f := range v.Frets {
		if f <= 0 {
			continue
		}
		if lo == 0 || int(f) < lo {
			lo = int(f)
		}
		if int(f) > hi {
			hi = int(f)
		}
	}
	if lo == 0 {
		return 0
	}
	return hi - lo + 1
}

// HasBarre reports whether the shape needs one.
func (v Voicing) HasBarre() bool { return len(v.Barres) > 0 }

// ID is a stable handle for one shape of one chord, for a deep link or a DOM id.
//
// It names the shape and its position rather than an index into the result list,
// so adding a shape to the dataset cannot silently repoint an existing link.
func (v Voicing) ID() string {
	if v.Position == 0 {
		return v.Shape
	}
	return v.Shape + "-" + strconv.Itoa(v.Position)
}

// ApproxKind says how a shape departs from the chord that was asked for.
type ApproxKind int

const (
	// Exact means the shape voices the chord as written.
	Exact ApproxKind = iota
	// BassOmitted means a slash bass was dropped. A slash bass is usually an
	// inversion somebody else is covering, so dropping it is more faithful than
	// changing the chord's quality.
	BassOmitted
	// QualitySimplified means the family was reduced to one that has shapes.
	QualitySimplified
	// ForeignTuning means a standard-tuning shape that happens to be valid in
	// this tuning because it does not touch the strings that differ.
	ForeignTuning
)

// Approximation records a departure as data rather than as a sentence, because
// both the diagram's title and the page need it and both will want translating.
type Approximation struct {
	Kind  ApproxKind
	Asked string // the chord as written
	Given string // the chord actually shown
}

// Exact reports whether nothing was given up.
func (a Approximation) Exact() bool { return a.Kind == Exact }

// Options tunes a lookup for one song.
type Options struct {
	Tuning Tuning // zero value means standard
	Capo   int    // reserved: version one always counts frets from the nut
	Max    int    // 0 means DefaultMax
}

// DefaultMax is how many shapes a lookup returns. Beyond four, the reader is
// choosing between positions they will never use.
const DefaultMax = 4

// Result is everything a view needs for one chord.
type Result struct {
	Symbol   string // the chord as displayed
	Quality  harmony.Quality
	Voicings []Voicing
	// Warning explains an empty or approximate result in Spanish, ready to show.
	Warning string
}

// Found reports whether there is anything to draw.
func (r Result) Found() bool { return len(r.Voicings) > 0 }

// Tuning is what the open strings sound.
//
// Open holds MIDI note numbers because the only things this package asks of a
// tuning are pitch class and pitch order, neither of which needs a spelling.
// Keeping spelling out also means two tunings can be compared for equality.
type Tuning struct {
	Name string
	Open []int
}

// Standard is EADGBE.
func Standard() Tuning {
	return Tuning{Name: "EADGBE", Open: []int{40, 45, 50, 55, 59, 64}}
}

// Strings is the number of strings the tuning describes.
func (t Tuning) Strings() int { return len(t.Open) }

// IsZero reports whether the tuning is unset.
func (t Tuning) IsZero() bool { return len(t.Open) == 0 }

// Equal reports whether two tunings sound the same, ignoring their names.
func (t Tuning) Equal(o Tuning) bool {
	if len(t.Open) != len(o.Open) {
		return false
	}
	for i := range t.Open {
		if t.Open[i] != o.Open[i] {
			return false
		}
	}
	return true
}

// knownTunings maps what people write in a song file onto real tunings.
//
// The song format promises that the tuning field is free text, so an
// unrecognised value must never be an error here — it simply means no diagrams,
// with an honest explanation.
var knownTunings = []struct {
	tuning  Tuning
	aliases []string
}{
	{Tuning{Name: "EADGBE", Open: []int{40, 45, 50, 55, 59, 64}},
		[]string{"eadgbe", "standard", "estandar", "afinacionestandar", "e"}},
	{Tuning{Name: "Drop D", Open: []int{38, 45, 50, 55, 59, 64}},
		[]string{"dropd", "dadgbe", "dropdtuning"}},
	{Tuning{Name: "Eb standard", Open: []int{39, 44, 49, 54, 58, 63}},
		[]string{"ebstandard", "ebadgbe", "halfstepdown", "mediotonoabajo", "eb"}},
	{Tuning{Name: "D standard", Open: []int{38, 43, 48, 53, 57, 62}},
		[]string{"dstandard", "wholestepdown", "tonoabajo", "dgcfad"}},
	{Tuning{Name: "DADGAD", Open: []int{38, 45, 50, 55, 57, 62}},
		[]string{"dadgad"}},
	{Tuning{Name: "Open G", Open: []int{38, 43, 50, 55, 59, 62}},
		[]string{"openg", "dgdgbd", "solabierto"}},
	{Tuning{Name: "Open D", Open: []int{38, 45, 50, 54, 57, 62}},
		[]string{"opend", "dadfad", "reabierto"}},
}

// ParseTuning interprets a song's tuning field.
//
// It reports false for anything it does not recognise, which is not an error:
// the field is free text by design, and "Drop D (ish)" must not stop a song from
// being read.
func ParseTuning(s string) (Tuning, bool) {
	key := tuningKey(s)
	if key == "" {
		return Standard(), true // no tuning stated means the usual one
	}
	for _, kt := range knownTunings {
		for _, a := range kt.aliases {
			if key == a {
				return kt.tuning, true
			}
		}
	}
	return Tuning{}, false
}

// tuningKey reduces a tuning name to a comparison key: lowercase letters and
// digits only, so "Drop-D", "drop d" and "DROPD" all match.
func tuningKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompatibleWith reports whether a shape written for the src tuning still sounds
// the same chord in dst, and is therefore safe to show.
//
// This is what makes Drop D nearly free. It differs from standard on the lowest
// string only, so every shape that mutes or omits that string is valid verbatim
// — which covers most of the useful ones. A shape that does sound the changed
// string is dropped rather than shown wrong.
func compatible(src, dst Tuning, frets []Fret) bool {
	if src.Equal(dst) {
		return true
	}
	if len(src.Open) != len(dst.Open) || len(frets) != len(src.Open) {
		return false
	}
	for i, f := range frets {
		if f == Muted {
			continue // not sounded, so its tuning does not matter
		}
		if src.Open[i] != dst.Open[i] {
			return false
		}
	}
	return true
}

// parseFrets reads a fret specification such as "x 0 2 0 1 0".
//
// The dataset is written this way rather than as arrays of numbers because a
// hand-curated dataset lives or dies on being reviewable, and "x32010" breaks
// the moment a fret reaches double figures. This is the same trade the .song
// format itself makes: a small strict syntax that refuses what it does not
// understand.
func parseFrets(spec string) ([]Fret, error) {
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty fret specification")
	}
	out := make([]Fret, 0, len(fields))
	for _, f := range fields {
		switch f {
		case "x", "X":
			out = append(out, Muted)
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("%q is not a fret", f)
		}
		if n < 0 || n > 24 {
			return nil, fmt.Errorf("fret %d is out of range", n)
		}
		out = append(out, Fret(n))
	}
	return out, nil
}

// parseFingers reads a finger specification such as "- - 2 - 1 -".
func parseFingers(spec string, want int) ([]Finger, error) {
	if strings.TrimSpace(spec) == "" {
		return make([]Finger, want), nil
	}
	fields := strings.Fields(spec)
	if len(fields) != want {
		return nil, fmt.Errorf("got %d fingers for %d strings", len(fields), want)
	}
	out := make([]Finger, 0, want)
	for _, f := range fields {
		switch f {
		case "-", "0":
			out = append(out, NoFinger)
			continue
		case "T", "t", "P", "p":
			out = append(out, Thumb)
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > 5 {
			return nil, fmt.Errorf("%q is not a finger", f)
		}
		out = append(out, Finger(n))
	}
	return out, nil
}
