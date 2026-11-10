package song

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Laying a section out as its words and what floats above them, and writing it
// back.
//
// An editor that draws each chord over its syllable holds the words in one place
// and the chords in another, and both are its own to move about. What it is not
// given is the format: which characters want an escape, what makes a bracket a
// chord, how long a fence has to be. It sends back words and positions, and the
// brackets are written here, by the code that writes every other line of a song
// file. One set of rules, in the one place that tests them.

// Row is one line of a section the way an editor lays it out, or one fenced
// block.
type Row struct {
	// Words is the line with its brackets taken out and its escapes resolved.
	// For a fenced block it is the lines inside the fence.
	Words string `json:"words"`
	Marks []Mark `json:"marks,omitempty"`

	Fenced bool   `json:"fenced,omitempty"`
	Info   string `json:"info,omitempty"` // the tag after the fence, for example "tab"
}

// Mark is something written in brackets, and where in the words it stands.
type Mark struct {
	// At is how far into the words the bracket stood, in UTF-16 code units.
	// That is how a browser counts along a string, and the browser is who moves
	// it; counted any other way, an emoji in front of a chord would shift it.
	At   int    `json:"at"`
	Text string `json:"text"`
	// Chord says the text reads as a chord and not as an annotation. It is told
	// to the editor and never taken from it: what a bracket holds is decided
	// when the line is written.
	Chord bool `json:"chord,omitempty"`
	// Typed says the text is a name somebody typed where a chord goes, which
	// has not been read yet. It is read here the way such a name is, so that
	// "lam" is written Am, and a song saved before the editor had heard what
	// the name meant is still saved right. It is taken from the editor and
	// never told to it.
	Typed bool `json:"typed,omitempty"`
}

// Rows lays a section's body out a line at a time.
//
// It reports an error for a body that cannot be shown this way: a bracket that
// never closes, a fence left open, a line that begins with "#". Those are
// mistakes in the text, and the place to mend one is the text.
func Rows(body string) ([]Row, error) {
	// A browser cannot hold bytes that are not text, and sends back something
	// else in their place.
	if !utf8.ValidString(body) {
		return nil, errors.New("the body is not valid UTF-8")
	}
	lines := strings.Split(newlines(body), "\n")
	rows := make([]Row, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		switch {
		case strings.HasPrefix(raw, "```"):
			marker := len(raw) - len(strings.TrimLeft(raw, "`"))
			info := strings.TrimSpace(raw[marker:])
			if strings.ContainsAny(info, "`") {
				return nil, lineErr(i, "parse.fencetick")
			}
			end := -1
			for j := i + 1; j < len(lines); j++ {
				if isFenceClose(strings.TrimRight(lines[j], " \t"), marker) {
					end = j
					break
				}
			}
			if end < 0 {
				return nil, lineErr(i, "parse.fenceopen")
			}
			rows = append(rows, Row{Words: strings.Join(lines[i+1:end], "\n"), Fenced: true, Info: info})
			i = end

		case strings.HasPrefix(raw, "#"):
			// A heading belongs between sections and not inside one, and any
			// other line that starts this way is one the format keeps for later.
			return nil, lineErr(i, "parse.hashreserved")

		case strings.TrimSpace(raw) == "":
			rows = append(rows, Row{})

		default:
			toks, err := tokenizeLine(raw, i)
			if err != nil {
				return nil, err
			}
			rows = append(rows, rowOf(toks))
		}
	}
	return rows, nil
}

func rowOf(toks []Token) Row {
	var row Row
	var words strings.Builder
	at := 0
	for _, t := range toks {
		switch t.Kind {
		case TokenChord:
			row.Marks = append(row.Marks, Mark{At: at, Text: t.Chord.String(), Chord: true})
		case TokenAnnotation:
			row.Marks = append(row.Marks, Mark{At: at, Text: t.Text})
		default:
			words.WriteString(t.Text)
			at += units(t.Text)
		}
	}
	row.Words = words.String()
	return row
}

// FromRows writes rows back as the body of a section.
//
// Nothing an editor sends can make it write a line that reads back as something
// else. A position past the end of the words is the end of them, a bracket with
// nothing in it is not written, and a fence is made as long as what it holds
// needs.
func FromRows(rows []Row) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.Fenced {
			lines = append(lines, formatLine(lineOf(row)))
			continue
		}
		text := newlines(row.Words)
		fence := fenceFor(text)
		lines = append(lines, fence+strings.TrimSpace(strip(row.Info, "`\r\n")))
		if text != "" {
			lines = append(lines, text)
		}
		lines = append(lines, fence)
	}
	return strings.Join(lines, "\n")
}

// lineOf puts each bracket back into the words at the place it stood.
func lineOf(row Row) Line {
	// A row is one line, whatever was sent.
	words := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(row.Words)

	type placed struct {
		at    int // in bytes, on a character boundary
		text  string
		typed bool
	}
	marks := make([]placed, 0, len(row.Marks))
	for _, m := range row.Marks {
		// A closing bracket or a line ending inside one would end it early.
		if text := strip(m.Text, "]\r\n"); strings.TrimSpace(text) != "" {
			marks = append(marks, placed{byteAt(words, m.At), text, m.Typed})
		}
	}
	// Stable, so two brackets at the same place stay in the order they came in.
	slices.SortStableFunc(marks, func(a, b placed) int { return a.at - b.at })

	ln := Line{Kind: LineLyric}
	done := 0
	for _, m := range marks {
		if m.at > done {
			ln.Tokens = append(ln.Tokens, Token{Kind: TokenText, Text: words[done:m.at]})
			done = m.at
		}
		c, err := chord.Parse(m.text)
		isChord := err == nil
		if !isChord && m.typed {
			c, isChord = TypedChord(m.text)
		}
		if isChord {
			ln.Tokens = append(ln.Tokens, Token{Kind: TokenChord, Chord: c})
		} else {
			ln.Tokens = append(ln.Tokens, Token{Kind: TokenAnnotation, Text: m.text})
		}
	}
	if done < len(words) {
		ln.Tokens = append(ln.Tokens, Token{Kind: TokenText, Text: words[done:]})
	}
	return ln
}

// units is the length of s as a browser counts it.
func units(s string) int {
	n := 0
	for _, r := range s {
		n += max(utf16.RuneLen(r), 1)
	}
	return n
}

// byteAt turns a position counted the browser's way into one in bytes. A
// position inside a character, between the two halves of an emoji, is the end of
// that character, so that nothing is ever cut in two.
func byteAt(s string, at int) int {
	n := 0
	for i, r := range s {
		if n >= at {
			return i
		}
		n += max(utf16.RuneLen(r), 1)
	}
	return len(s)
}

// strip removes every one of the given characters from s.
func strip(s, chars string) string {
	if !strings.ContainsAny(s, chars) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(chars, r) {
			return -1
		}
		return r
	}, s)
}
