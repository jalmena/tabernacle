package chord

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotAChord reports text that is not a chord symbol.
//
// Callers are expected to treat this as ordinary information rather than a
// failure: bracketed text in a song file is not necessarily a chord, and
// "[Chorus]" or "[x2]" must survive as an annotation.
var ErrNotAChord = errors.New("not a chord symbol")

// Chord is a parsed chord symbol.
//
// Suffix holds the quality, extensions and alterations verbatim, which is what
// makes formatting lossless and transposition safe: moving a chord only ever
// touches Root and Bass.
type Chord struct {
	Root    Note
	Suffix  string
	Bass    Note // valid only when HasBass is true
	HasBass bool
}

// Parse parses a chord symbol written in English notation.
func Parse(s string) (Chord, error) { return ParseIn(English, s) }

// ParseIn parses a chord symbol written in the given notation.
//
// Root note names must be capitalised. Lowercase-means-minor is a legacy
// convention that cannot be told apart from ordinary words, and guessing wrong
// would turn a lyric annotation into a chord the app then transposes.
func ParseIn(no Notation, s string) (Chord, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Chord{}, fmt.Errorf("empty chord symbol: %w", ErrNotAChord)
	}
	root, rest, ok := parseNote(no, s)
	if !ok {
		return Chord{}, fmt.Errorf("%q: %w", s, ErrNotAChord)
	}
	c := Chord{Root: root}

	// A slash is only a slash bass when what follows it is a note on its own.
	// Testing that keeps suffixes that contain a slash, such as "6/9", intact.
	if i := strings.LastIndexByte(rest, '/'); i >= 0 {
		if bass, tail, ok := parseNote(no, rest[i+1:]); ok && tail == "" {
			c.Bass, c.HasBass = bass, true
			rest = rest[:i]
		}
	}

	if !validSuffix(rest) {
		return Chord{}, fmt.Errorf("%q: %w", s, ErrNotAChord)
	}
	c.Suffix = rest
	return c, nil
}

// suffixTokens are the pieces a chord quality suffix may be built from, listed
// longest first so that greedy matching prefers "maj" over "m".
//
// Validating suffixes at all is the whole point of this list. Without it,
// "[Chorus]" parses as C with the suffix "horus" and a transposition turns it
// into "Dhorus". The list is intentionally loose about musical nonsense such
// as "Cmsus2sus4" and strict about anything that looks like a word.
var suffixTokens = []string{
	"maj", "min", "dim", "aug", "sus", "add", "alt", "omit", "no",
	"13", "11", "9", "7", "6", "5", "4", "3", "2",
	"m", "M", "b", "#", "♭", "♯", "+", "-", "°", "Δ", "ø", "(", ")", "/",
}

func validSuffix(s string) bool {
	for s != "" {
		matched := false
		for _, t := range suffixTokens {
			if strings.HasPrefix(s, t) {
				s = s[len(t):]
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Format writes the chord in the given notation. Only note names change; the
// quality suffix is reproduced verbatim, so a chord read from a file is
// written back exactly as the user typed it.
func (c Chord) Format(no Notation) string {
	var b strings.Builder
	b.WriteString(c.Root.Format(no))
	b.WriteString(c.Suffix)
	if c.HasBass {
		b.WriteByte('/')
		b.WriteString(c.Bass.Format(no))
	}
	return b.String()
}

// String formats the chord in English notation.
func (c Chord) String() string { return c.Format(English) }

// IsMinor reports whether the chord's quality is minor.
//
// This is a practical test on the suffix rather than a harmonic analysis: it
// exists so that transposition can pick the accidentals a minor key is
// conventionally written with.
func (c Chord) IsMinor() bool {
	switch {
	case strings.HasPrefix(c.Suffix, "min"):
		return true
	case strings.HasPrefix(c.Suffix, "maj"):
		return false
	case strings.HasPrefix(c.Suffix, "m"):
		return true // covers m, m7, mmaj7 and friends
	case strings.HasPrefix(c.Suffix, "-"):
		return true
	}
	return false
}
