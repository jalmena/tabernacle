package song

import (
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Chords returns the distinct chords the song uses, in the order they first
// appear.
//
// This is why a .song file never carries a list of its own chords: the list is
// derived, so it cannot drift out of step with the song. Order of first
// appearance is deliberate — it is the order a player meets them in, which is
// what makes a "chords used" panel worth reading before starting.
//
// Chords are compared by their canonical English spelling, so the same chord
// written twice counts once, while Cadd9 and C stay separate because they are
// different shapes.
func (s Song) Chords() []chord.Chord {
	var out []chord.Chord
	seen := make(map[string]bool)
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			for _, tok := range ln.Tokens {
				if tok.Kind != TokenChord {
					continue
				}
				key := tok.Chord.String()
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, tok.Chord)
			}
		}
	}
	return out
}

// PlainText returns the song's words with the chords taken out, one line per
// line of the song.
//
// Verbatim blocks are left out on purpose. They hold tablature, and indexing
// "e|---0---3---|" would fill a search index with fretboard art that nobody
// searches for while diluting the ranking of the words that they do.
func (s Song) PlainText() string {
	var b strings.Builder
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			if ln.Kind != LineLyric {
				continue
			}
			start := b.Len()
			for _, tok := range ln.Tokens {
				if tok.Kind == TokenText {
					b.WriteString(tok.Text)
				}
			}
			// Skip a line that was nothing but chords: it contributes no words.
			if b.Len() == start {
				continue
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// CountLines returns how many lines of words the song has, ignoring blank lines
// and verbatim blocks. It is a rough measure of length, used to show how long a
// song is before opening it.
func (s Song) CountLines() int {
	n := 0
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			if ln.Kind == LineLyric {
				n++
			}
		}
	}
	return n
}
