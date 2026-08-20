// Package harmony interprets chord symbols.
//
// It exists as its own package because internal/chord deliberately treats a
// quality suffix as opaque, and that opacity is load-bearing: it is why
// "Cmaj7#11" survives a transposition untouched instead of being mangled into
// something unplayable. Interpretation is a separate concern, so it lives
// separately, and internal/chord stays a parser.
//
// This is not a music theory engine either. It answers exactly two questions,
// because those are the two the application actually asks: what family of chord
// is this, so a guitar shape can be looked up; and what does it simplify to,
// when no shape exists for it.
package harmony

import (
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Quality is a chord family, as a closed set.
type Quality int

const (
	Unknown Quality = iota
	Major
	Minor
	Dominant7
	Major7
	Minor7
	MinorMajor7
	Diminished
	Diminished7
	HalfDiminished // m7b5
	Augmented
	Sus2
	Sus4
	Dominant7Sus4
	Major6
	Minor6
	Add9
	Dominant9
	Major9
	Minor9
	Power // a root and a fifth, no third
)

// qualityInfo is everything known about one family.
type qualityInfo struct {
	symbol    string // canonical display suffix
	name      string // Spanish name, for a diagram caption
	intervals []int  // semitones above the root
	parent    Quality
}

// table is the whole of this package's theory.
//
// intervals exist so that the voicing dataset can be checked rather than
// trusted: a test can confirm that "x 0 2 0 1 0" really does spell Am7 instead
// of hoping the fret numbers were typed correctly.
//
// parent is the simplification chain, as data rather than as a switch, so a test
// can walk the whole enum and prove every family reaches Major or Minor without
// a cycle. A new family with a missing parent then fails the build.
var table = map[Quality]qualityInfo{
	Major:          {"", "mayor", []int{0, 4, 7}, Unknown},
	Minor:          {"m", "menor", []int{0, 3, 7}, Unknown},
	Dominant7:      {"7", "séptima", []int{0, 4, 7, 10}, Major},
	Major7:         {"maj7", "séptima mayor", []int{0, 4, 7, 11}, Major},
	Minor7:         {"m7", "menor séptima", []int{0, 3, 7, 10}, Minor},
	MinorMajor7:    {"m(maj7)", "menor con séptima mayor", []int{0, 3, 7, 11}, Minor},
	Diminished:     {"dim", "disminuido", []int{0, 3, 6}, Minor},
	Diminished7:    {"dim7", "disminuido séptima", []int{0, 3, 6, 9}, Diminished},
	HalfDiminished: {"m7b5", "semidisminuido", []int{0, 3, 6, 10}, Minor7},
	Augmented:      {"aug", "aumentado", []int{0, 4, 8}, Major},
	Sus2:           {"sus2", "suspendido segunda", []int{0, 2, 7}, Major},
	Sus4:           {"sus4", "suspendido cuarta", []int{0, 5, 7}, Major},
	Dominant7Sus4:  {"7sus4", "séptima suspendida", []int{0, 5, 7, 10}, Dominant7},
	Major6:         {"6", "sexta", []int{0, 4, 7, 9}, Major},
	Minor6:         {"m6", "menor sexta", []int{0, 3, 7, 9}, Minor},
	Add9:           {"add9", "con novena añadida", []int{0, 4, 7, 14}, Major},
	Dominant9:      {"9", "novena", []int{0, 4, 7, 10, 14}, Dominant7},
	Major9:         {"maj9", "novena mayor", []int{0, 4, 7, 11, 14}, Major7},
	Minor9:         {"m9", "menor novena", []int{0, 3, 7, 10, 14}, Minor7},
	Power:          {"5", "quinta", []int{0, 7}, Major},
}

// Symbol returns the canonical way to write this quality.
func (q Quality) Symbol() string { return table[q].symbol }

// Name returns the quality's name in Spanish, for a caption.
func (q Quality) Name() string { return table[q].name }

// Intervals returns the semitones above the root that the quality contains.
func (q Quality) Intervals() []int {
	src := table[q].intervals
	out := make([]int, len(src))
	copy(out, src)
	return out
}

// Parent returns the family this one simplifies to, and false when it is
// already as simple as it gets.
func (q Quality) Parent() (Quality, bool) {
	p := table[q].parent
	return p, p != Unknown
}

// Known reports whether the quality is one this package understands.
func (q Quality) Known() bool { _, ok := table[q]; return ok }

// All returns every known quality, for tests and for the dataset validator.
func All() []Quality {
	out := make([]Quality, 0, len(table))
	for q := range table {
		out = append(out, q)
	}
	return out
}

// Reduction records decoration that was stripped in order to classify a symbol.
//
// It is kept separate from simplification: a reduction is a property of the
// written symbol ("13#11" is a thirteenth with an alteration this package does
// not model), while a simplification is a property of the dataset (no shape
// exists for a ninth, so a seventh is offered instead). Conflating the two makes
// it impossible to tell the reader honestly what they are looking at.
type Reduction struct {
	// Dropped is the part of the suffix that was ignored, empty when the symbol
	// was understood exactly.
	Dropped string
}

// Exact reports whether nothing had to be ignored.
func (r Reduction) Exact() bool { return r.Dropped == "" }

// aliases maps the ways guitarists actually write a quality onto the families
// above. It is deliberately generous: the same chord turns up spelled half a
// dozen ways across the sheets people paste in.
var aliases = map[string]Quality{
	"":        Major,
	"maj":     Major,
	"M":       Major,
	"m":       Minor,
	"min":     Minor,
	"-":       Minor,
	"7":       Dominant7,
	"dom7":    Dominant7,
	"maj7":    Major7,
	"M7":      Major7,
	"Maj7":    Major7,
	"Δ":       Major7,
	"Δ7":      Major7,
	"m7":      Minor7,
	"min7":    Minor7,
	"-7":      Minor7,
	"mMaj7":   MinorMajor7,
	"m(maj7)": MinorMajor7,
	"mmaj7":   MinorMajor7,
	"minmaj7": MinorMajor7,
	"dim":     Diminished,
	"°":       Diminished,
	"o":       Diminished,
	"dim7":    Diminished7,
	"°7":      Diminished7,
	"m7b5":    HalfDiminished,
	"m7-5":    HalfDiminished,
	"ø":       HalfDiminished,
	"ø7":      HalfDiminished,
	"min7b5":  HalfDiminished,
	"aug":     Augmented,
	"+":       Augmented,
	"aug5":    Augmented,
	"sus2":    Sus2,
	"sus9":    Sus2,
	"2":       Sus2,
	"sus":     Sus4,
	"sus4":    Sus4,
	"4":       Sus4,
	"7sus4":   Dominant7Sus4,
	"7sus":    Dominant7Sus4,
	"6":       Major6,
	"maj6":    Major6,
	"M6":      Major6,
	"m6":      Minor6,
	"min6":    Minor6,
	"-6":      Minor6,
	"add9":    Add9,
	"add2":    Add9,
	"9":       Dominant9,
	"dom9":    Dominant9,
	"maj9":    Major9,
	"M9":      Major9,
	"m9":      Minor9,
	"min9":    Minor9,
	"-9":      Minor9,
	"5":       Power,
	"no3":     Power,
	"omit3":   Power,
}

// implied covers the symbols that name a family plus an extension the family
// does not itself contain.
//
// These cannot be handled by peeling, because peeling "13" off "13" leaves
// nothing, and nothing means a plain major triad — which is the wrong answer for
// a dominant chord. Naming them explicitly keeps the classification musical and
// still records the extension as dropped, so the page can say a simplified shape
// is being shown.
var implied = map[string]struct {
	quality   Quality
	extension string
}{
	"11":     {Dominant7, "11"},
	"13":     {Dominant7, "13"},
	"m11":    {Minor7, "11"},
	"min11":  {Minor7, "11"},
	"-11":    {Minor7, "11"},
	"m13":    {Minor7, "13"},
	"min13":  {Minor7, "13"},
	"maj11":  {Major7, "11"},
	"maj13":  {Major7, "13"},
	"M13":    {Major7, "13"},
	"6/9":    {Major6, "/9"},
	"m6/9":   {Minor6, "/9"},
	"7b5":    {Dominant7, "b5"},
	"7#5":    {Dominant7, "#5"},
	"7b9":    {Dominant7, "b9"},
	"7#9":    {Dominant7, "#9"},
	"7#11":   {Dominant7, "#11"},
	"7b13":   {Dominant7, "b13"},
	"9b5":    {Dominant9, "b5"},
	"9#11":   {Dominant9, "#11"},
	"maj7#5": {Major7, "#5"},
	"m7#5":   {Minor7, "#5"},
}

// decorations are the trailing alterations and extensions that can be dropped
// to reach a family this package models, longest first so that "#11" is tried
// before "11".
//
// Dropping an alteration is a real loss and is always reported, never hidden:
// showing a Cmaj7 shape for Cmaj7#11 is useful to a guitarist, but only if the
// page says so.
var decorations = []string{
	"(add9)", "(maj7)", "(b13)", "(#11)", "(b9)", "(#9)", "(b5)", "(#5)",
	"add11", "add13", "add9", "omit5", "sus4", "sus2",
	"#11", "b13", "#13", "b11", "#9", "b9", "#5", "b5", "13", "11", "6", "9", "4", "2",
}

// Of classifies a chord's quality.
//
// It returns false when nothing recognisable is left, in which case the caller
// must not invent a shape: no diagram is better than a wrong one.
func Of(c chord.Chord) (Quality, Reduction, bool) {
	s := normalize(c.Suffix)
	if q, ok := aliases[s]; ok {
		return q, Reduction{}, true
	}
	if imp, ok := implied[s]; ok {
		return imp.quality, Reduction{Dropped: imp.extension}, true
	}

	// Peel decorations off the end until what remains is a family we model.
	var dropped []string
	rest := s
	for range decorations {
		cut := ""
		for _, d := range decorations {
			if len(d) < len(rest) && strings.HasSuffix(rest, d) {
				cut = d
				break
			}
		}
		if cut == "" {
			break
		}
		rest = strings.TrimSuffix(rest, cut)
		dropped = append([]string{cut}, dropped...)
		if q, ok := aliases[rest]; ok {
			return q, Reduction{Dropped: strings.Join(dropped, "")}, true
		}
	}
	return Unknown, Reduction{}, false
}

// normalizer folds the typographic accidental signs onto ASCII, so that a
// suffix typed with ♭ or ♯ classifies the same as one typed with b or #.
var normalizer = strings.NewReplacer("♭", "b", "♯", "#")

// normalize prepares a suffix for lookup.
//
// Nothing is lowercased: "CM7" and "Cmaj7" are the same chord while "Cm7" is a
// different one, so case is meaning here, not style.
func normalize(suffix string) string {
	return normalizer.Replace(strings.TrimSpace(suffix))
}

// Simplify rewrites a chord as the nearest family that has shapes, keeping the
// root and dropping the decoration.
//
// It reports false when the chord is already simple or cannot be classified.
// The caller is expected to say plainly that a substitution happened: a whole
// sheet simplified at once and labelled is a useful thing to hand a player,
// while a single chord silently swapped is a lie.
func Simplify(c chord.Chord) (chord.Chord, bool) {
	q, red, ok := Of(c)
	if !ok {
		return c, false
	}
	// An unrecognised decoration is itself the thing worth dropping.
	if !red.Exact() {
		out := c
		out.Suffix = q.Symbol()
		return out, out.Suffix != c.Suffix
	}
	parent, has := q.Parent()
	if !has {
		return c, false
	}
	out := c
	out.Suffix = parent.Symbol()
	return out, out.Suffix != c.Suffix
}
