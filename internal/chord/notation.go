package chord

import "strings"

// Notation selects how note names are written.
//
// English is the canonical notation: song files always store note names as C
// to B, so that a collection stays readable and diffable regardless of which
// notation the reader happens to prefer. Solfege exists for display and for
// importing sheets that were typed that way.
type Notation int

const (
	// English writes note names as C, D, E, F, G, A, B.
	English Notation = iota
	// Solfege writes note names as Do, Re, Mi, Fa, Sol, La, Si, the
	// convention used in Spanish, Italian and French.
	Solfege
)

// solfegeName maps 'A' to 'G' onto solfege syllables.
var solfegeName = [7]string{"La", "Si", "Do", "Re", "Mi", "Fa", "Sol"}

// solfegeOrder lists syllables longest first so that greedy matching prefers
// "Sol" over "Si".
var solfegeOrder = []struct {
	name   string
	letter byte
}{
	{"Sol", 'G'},
	{"Do", 'C'},
	{"Re", 'D'},
	{"Mi", 'E'},
	{"Fa", 'F'},
	{"La", 'A'},
	{"Si", 'B'},
}

// Format writes the note in the given notation.
func (n Note) Format(no Notation) string {
	if !n.Valid() {
		return ""
	}
	name := string(n.Letter)
	if no == Solfege {
		name = solfegeName[n.Letter-'A']
	}
	return name + accidentalString(n.Accidental)
}

// parseNote reads a note name from the front of s and returns the note and the
// unconsumed remainder.
//
// Accidentals are consumed greedily, so "Bb5" is a Bb power chord rather than
// a B with a flattened fifth. That is the universal convention on chord
// sheets; write "B(b5)" for the other reading.
func parseNote(no Notation, s string) (Note, string, bool) {
	var n Note
	switch no {
	case Solfege:
		for _, c := range solfegeOrder {
			if strings.HasPrefix(s, c.name) {
				n.Letter = c.letter
				s = s[len(c.name):]
				break
			}
		}
		if n.Letter == 0 {
			return Note{}, s, false
		}
	default:
		if s == "" || s[0] < 'A' || s[0] > 'G' {
			return Note{}, s, false
		}
		n.Letter = s[0]
		s = s[1:]
	}
	n.Accidental, s = parseAccidental(s)
	return n, s, true
}

// parseAccidental consumes a run of up to two accidentals of the same kind.
func parseAccidental(s string) (int, string) {
	acc := 0
	for acc > -2 && acc < 2 {
		switch {
		case strings.HasPrefix(s, "#"), strings.HasPrefix(s, "♯"):
			if acc < 0 {
				return acc, s
			}
			acc++
		case strings.HasPrefix(s, "b"), strings.HasPrefix(s, "♭"):
			if acc > 0 {
				return acc, s
			}
			acc--
		default:
			return acc, s
		}
		if s[0] == '#' || s[0] == 'b' {
			s = s[1:]
		} else {
			s = s[3:] // the sharp and flat runes are three bytes in UTF-8
		}
	}
	return acc, s
}
