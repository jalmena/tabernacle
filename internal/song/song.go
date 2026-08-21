// Package song holds the Tabernacle domain model for a song sheet together
// with the reader and writer for the .song file format.
//
// Song files are the source of truth for a collection; the database is only an
// index that can be rebuilt from them. Nothing here knows about HTTP, SQL or
// the filesystem, which is what keeps the format and the musical rules
// testable on their own.
package song

import (
	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/tab"
)

// retiredFields are metadata fields the format used to carry and no longer
// does. They are accepted and ignored rather than rejected, so a collection
// written before they were dropped still reads without complaint.
//
// "format" carried a schema version. It went because a version nobody bumps is
// a field everybody types; if versioning is ever needed again, its absence can
// simply mean the first version. "tempo" and "meter" went because neither
// changes anything a guitarist reads off the page. "tags" went because a free
// list of labels is a filing system nobody maintains; one genre is the part
// people actually use.
var retiredFields = map[string]bool{"format": true, "tempo": true, "meter": true, "tags": true}

// Song is one song sheet.
type Song struct {
	Title    string
	Artist   string
	Key      string // chord symbol, for example "G" or "F#m"
	Capo     int    // fret the capo sits on, 0 for none
	Tuning   string // free text, for example "EADGBE" or "Drop D"
	Genre    string // one label, for example "rock" or "bolero"
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
	out, _ := s.TransposeReport(semitones)
	return out
}

// TransposeReport is what a transposition could and could not do.
//
// It exists because tablature has a floor and a ceiling that chord symbols do
// not: there is nothing below an open string, and the neck runs out. A block that
// cannot move is left exactly as it was, and the caller is told, so a sheet never
// shows a solo that silently disagrees with the chords above it.
type TransposeReport struct {
	TabsShifted int
	TabsTooLow  int
	TabsTooHigh int
}

// TransposeReport moves the song and says what happened to its tablature.
func (s Song) TransposeReport(semitones int) (Song, TransposeReport) {
	var rep TransposeReport
	out := s.clone()
	if semitones%12 == 0 {
		return out, rep
	}
	key, ok := s.keyChord()
	if !ok {
		return out, rep
	}
	newKey, spelling := chord.TransposeKey(key, semitones)
	if s.Key != "" {
		out.Key = newKey.String()
	}
	for i := range out.Sections {
		for j := range out.Sections[i].Lines {
			ln := &out.Sections[i].Lines[j]
			switch ln.Kind {
			case LineVerbatim:
				// Tablature moves with the chords, or the solo ends up in a
				// different key from the song around it.
				shifted, res := tab.Shift(ln.Info, ln.Text, semitones)
				switch {
				case res.Shifted:
					ln.Text = shifted
					rep.TabsShifted++
				case res.TooLow:
					rep.TabsTooLow++
				case res.TooHigh:
					rep.TabsTooHigh++
				}
			default:
				for k := range ln.Tokens {
					if ln.Tokens[k].Kind == TokenChord {
						ln.Tokens[k].Chord = ln.Tokens[k].Chord.Transpose(semitones, spelling)
					}
				}
			}
		}
	}
	return out, rep
}

// MapChords returns a copy of the song with every chord replaced by fn.
//
// The copy is deep, so the caller's song is never touched. The key field is left
// alone: it names what key the sheet is in, not a chord in the words, and
// rewriting it would change what the sheet claims about itself.
func (s Song) MapChords(fn func(chord.Chord) chord.Chord) Song {
	out := s.clone()
	for i := range out.Sections {
		for j := range out.Sections[i].Lines {
			toks := out.Sections[i].Lines[j].Tokens
			for k := range toks {
				if toks[k].Kind == TokenChord {
					toks[k].Chord = fn(toks[k].Chord)
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
