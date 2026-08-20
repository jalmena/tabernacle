// Package chord parses, formats and transposes the chord symbols a guitarist
// actually writes on a chord sheet.
//
// It is deliberately not a music-theory engine. A chord is a root note, an
// opaque but validated quality suffix, and an optional slash bass. That is
// everything transposition needs, and it means an exotic suffix this package
// does not understand survives a transposition untouched instead of being
// mangled into something unplayable.
package chord

import "strings"

// Note is a pitch class together with the way it is spelled.
//
// Spelling is kept alongside pitch because parsing must be lossless: Gb and F#
// sound the same, but silently rewriting one into the other would churn the
// user's files on every save and produce diffs nobody asked for.
type Note struct {
	Letter     byte // 'A' to 'G'
	Accidental int  // semitones applied to Letter, -2 to +2
}

// pitchOfLetter maps 'A' to 'G' onto semitones above C.
var pitchOfLetter = [7]int{9, 11, 0, 2, 4, 5, 7}

// Valid reports whether the note has a usable letter name. The zero Note is
// not valid; it is what a Chord carries in its Bass field when HasBass is
// false.
func (n Note) Valid() bool {
	return n.Letter >= 'A' && n.Letter <= 'G' && n.Accidental >= -2 && n.Accidental <= 2
}

// PitchClass returns the pitch class of the note, 0 for C through 11 for B.
// It reports 0 for an invalid note.
func (n Note) PitchClass() int {
	if !n.Valid() {
		return 0
	}
	pc := (pitchOfLetter[n.Letter-'A'] + n.Accidental) % 12
	if pc < 0 {
		pc += 12
	}
	return pc
}

// String formats the note in English notation, for example "Bb" or "F#".
func (n Note) String() string { return n.Format(English) }

func accidentalString(a int) string {
	switch {
	case a < 0:
		return strings.Repeat("b", -a)
	case a > 0:
		return strings.Repeat("#", a)
	}
	return ""
}
