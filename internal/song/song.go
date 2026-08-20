// Package song holds the Tabernacle domain model for a song sheet together
// with the reader and writer for the .song file format.
//
// Song files are the source of truth for a collection; the database is only an
// index that can be rebuilt from them. Nothing here knows about HTTP, SQL or
// the filesystem, which is what keeps the format and the musical rules
// testable on their own.
package song

import "github.com/jalmena/tabernacle/internal/chord"

// Format is the .song format version this package reads and writes.
const Format = 1

// Song is one song sheet.
type Song struct {
	Format   int
	Title    string
	Artist   string
	Key      string // chord symbol, for example "G" or "F#m"
	Capo     int    // fret the capo sits on, 0 for none
	Tuning   string // free text, for example "EADGBE" or "Drop D"
	Tempo    int    // beats per minute, 0 when unknown
	Meter    string // time signature, for example "4/4"
	Tags     []string
	Notes    string
	Sections []Section
}

// Section is one labelled part of a sheet, such as a verse or a chorus.
type Section struct {
	Kind  Kind
	Label string // exactly as the user wrote it, for example "Verse 2"
	Lines []Line
}

// LineKind distinguishes the three things a line of a sheet can be.
type LineKind int

const (
	// LineLyric is a line of text with chords positioned inside it.
	LineLyric LineKind = iota
	// LineBlank is an empty line. Blank lines are kept because guitarists use
	// them to group stanzas, and print layout depends on that grouping.
	LineBlank
	// LineVerbatim is a fenced block reproduced exactly: tablature, a riff,
	// anything that depends on monospaced alignment. Its Text field holds the
	// whole block, newlines included, because a fenced block is one visual
	// unit rather than a run of independent lines.
	LineVerbatim
)

// Line is one line of a section, or one whole fenced block when Kind is
// LineVerbatim.
type Line struct {
	Kind   LineKind
	Tokens []Token // LineLyric only
	Text   string  // LineVerbatim only
	Info   string  // LineVerbatim only: the fence tag, for example "tab"
}

// TokenKind distinguishes the three things bracketed text can turn out to be.
type TokenKind int

const (
	// TokenText is literal lyric text.
	TokenText TokenKind = iota
	// TokenChord is a chord symbol that was parsed successfully.
	TokenChord
	// TokenAnnotation is bracketed text that is not a chord, such as "[x2]".
	// It is kept verbatim and never transposed.
	TokenAnnotation
)

// Token is one piece of a lyric line.
type Token struct {
	Kind  TokenKind
	Text  string      // TokenText and TokenAnnotation
	Chord chord.Chord // TokenChord only
}

// Transpose returns a copy of s with every chord moved by semitones.
//
// The declared key decides how the result is spelled, so a sheet moved into Eb
// reads in flats throughout instead of mixing A# and Bb. A song that declares
// no key falls back to its first chord, which is the same guess a guitarist
// makes when handed an unlabelled sheet.
//
// Capo is deliberately left alone: it describes what the player does with a
// piece of metal, not what the sheet says.
func (s Song) Transpose(semitones int) Song {
	out := s.clone()
	if semitones%12 == 0 {
		return out
	}
	key, ok := s.keyChord()
	if !ok {
		return out
	}
	newKey, spelling := chord.TransposeKey(key, semitones)
	if s.Key != "" {
		out.Key = newKey.String()
	}
	for i := range out.Sections {
		for j := range out.Sections[i].Lines {
			toks := out.Sections[i].Lines[j].Tokens
			for k := range toks {
				if toks[k].Kind == TokenChord {
					toks[k].Chord = toks[k].Chord.Transpose(semitones, spelling)
				}
			}
		}
	}
	return out
}

// keyChord returns the chord the sheet's spelling should be derived from.
func (s Song) keyChord() (chord.Chord, bool) {
	if s.Key != "" {
		if c, err := chord.Parse(s.Key); err == nil {
			return c, true
		}
	}
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			for _, tok := range ln.Tokens {
				if tok.Kind == TokenChord {
					return tok.Chord, true
				}
			}
		}
	}
	return chord.Chord{}, false
}

// clone returns a deep copy so that Transpose cannot alias the caller's song.
func (s Song) clone() Song {
	out := s
	out.Tags = append([]string(nil), s.Tags...)
	out.Sections = make([]Section, len(s.Sections))
	for i, sec := range s.Sections {
		out.Sections[i] = sec
		out.Sections[i].Lines = make([]Line, len(sec.Lines))
		for j, ln := range sec.Lines {
			out.Sections[i].Lines[j] = ln
			out.Sections[i].Lines[j].Tokens = append([]Token(nil), ln.Tokens...)
		}
	}
	return out
}
