package song

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Parse reads a .song file.
//
// Imported files are untrusted input: every failure is reported with the line
// it happened on, and no input is silently reinterpreted. Bracketed text that
// is not a chord becomes an annotation rather than an error, because "[x2]"
// and "[Chorus]" are things people really write.
func Parse(data []byte) (Song, error) {
	// Editors on Windows and some phone apps prepend a byte order mark.
	text := strings.TrimPrefix(string(data), "\ufeff")

	// Every line ending becomes a newline: CRLF from Windows, and a lone CR,
	// which is both classic Mac and what a stray keystroke leaves behind. A
	// carriage return is a line ending in every case and never content, so
	// keeping one would put an invisible control character inside a lyric and
	// then carry it into filenames and pages.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	entries, bodyStart, err := parseFrontMatter(lines)
	if err != nil {
		return Song{}, err
	}
	var s Song
	if err := decodeFrontMatter(entries, &s); err != nil {
		return Song{}, err
	}
	if s.Sections, err = parseBody(lines, bodyStart); err != nil {
		return Song{}, err
	}
	if err := Validate(s); err != nil {
		return Song{}, err
	}
	return s, nil
}

func decodeFrontMatter(entries []entry, s *Song) error {
	for _, e := range entries {
		var err error
		switch e.key {
		case "title":
			s.Title, err = e.stringValue()
		case "artist":
			s.Artist, err = e.stringValue()
		case "key":
			if s.Key, err = e.stringValue(); err == nil {
				err = validateKey(s.Key)
			}
		case "capo":
			if s.Capo, err = e.intValue(); err == nil {
				err = validateCapo(s.Capo)
			}
		case "tuning":
			s.Tuning, err = e.stringValue()
		case "genre":
			if s.Genre, err = e.stringValue(); err == nil {
				err = validateGenre(s.Genre)
			}
		case "notes":
			s.Notes, err = e.stringValue()
		default:
			if !retiredFields[e.key] {
				err = fmt.Errorf("unknown field %q", e.key)
			}
			// A retired field is read and discarded. Rejecting it would make a
			// collection written before it was dropped unreadable, and the
			// point of an unknown-field error is to catch a typo, not to punish
			// somebody for having older files.
		}
		if err != nil {
			return lineErr(e.line, err.Error())
		}
	}
	return nil
}

func (e entry) stringValue() (string, error) {
	if e.isList {
		return "", fmt.Errorf("field %q takes a single value, not a list", e.key)
	}
	return e.scalar, nil
}

func (e entry) intValue() (int, error) {
	if e.isList {
		return 0, fmt.Errorf("field %q takes a number, not a list", e.key)
	}
	n, err := strconv.Atoi(strings.TrimSpace(e.scalar))
	if err != nil {
		return 0, fmt.Errorf("field %q takes a whole number, got %q", e.key, e.scalar)
	}
	return n, nil
}

// parseBody reads the sections of a song file.
//
// Content that appears before the first heading is kept in an unlabelled
// section instead of being rejected: pasting a short song in as one block is a
// perfectly reasonable thing to do, and it round-trips.
func parseBody(lines []string, start int) ([]Section, error) {
	var sections []Section
	cur := -1
	ensure := func() {
		if cur < 0 {
			sections = append(sections, Section{Kind: KindOther})
			cur = len(sections) - 1
		}
	}

	for i := start; i < len(lines); i++ {
		raw := lines[i]
		switch {
		case strings.HasPrefix(raw, "```"):
			marker := len(raw) - len(strings.TrimLeft(raw, "`"))
			info := strings.TrimSpace(raw[marker:])
			if strings.ContainsAny(info, "`") {
				return nil, lineErr(i, "a fence tag cannot contain a backtick")
			}
			var body []string
			closed := false
			for j := i + 1; j < len(lines); j++ {
				if isFenceClose(strings.TrimRight(lines[j], " \t"), marker) {
					i, closed = j, true
					break
				}
				body = append(body, lines[j])
			}
			if !closed {
				return nil, lineErr(i, "unterminated fenced block")
			}
			ensure()
			sections[cur].Lines = append(sections[cur].Lines, Line{
				Kind: LineVerbatim,
				Text: strings.Join(body, "\n"),
				Info: info,
			})

		case strings.HasPrefix(raw, "## "):
			label := strings.TrimSpace(raw[3:])
			if label == "" {
				return nil, lineErr(i, "section heading needs a label")
			}
			sections = append(sections, Section{Kind: KindForLabel(label), Label: label})
			cur = len(sections) - 1

		case strings.HasPrefix(raw, "##"):
			return nil, lineErr(i, `section headings are written as "## Label"`)

		case strings.HasPrefix(raw, "#"):
			return nil, lineErr(i, `a body line starting with "#" is reserved; write \# for a literal #`)

		case strings.TrimSpace(raw) == "":
			if cur >= 0 {
				sections[cur].Lines = append(sections[cur].Lines, Line{Kind: LineBlank})
			}

		default:
			toks, err := tokenizeLine(raw, i)
			if err != nil {
				return nil, err
			}
			ensure()
			sections[cur].Lines = append(sections[cur].Lines, Line{Kind: LineLyric, Tokens: toks})
		}
	}

	// Blank lines at the edges of a section carry no meaning and would
	// otherwise make writing a file back a lossy operation.
	for i := range sections {
		ls := sections[i].Lines
		for len(ls) > 0 && ls[0].Kind == LineBlank {
			ls = ls[1:]
		}
		for len(ls) > 0 && ls[len(ls)-1].Kind == LineBlank {
			ls = ls[:len(ls)-1]
		}
		if len(ls) == 0 {
			ls = nil // an empty section and a trimmed-empty one are the same song
		}
		sections[i].Lines = ls
	}
	return sections, nil
}

// isFenceClose reports whether s is a run of at least marker backticks and
// nothing else.
func isFenceClose(s string, marker int) bool {
	n := len(s) - len(strings.TrimLeft(s, "`"))
	return n == len(s) && n >= marker
}

// tokenizeLine splits a lyric line into text, chords and annotations.
func tokenizeLine(text string, line int) ([]Token, error) {
	var toks []Token
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			toks = append(toks, Token{Kind: TokenText, Text: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(text); {
		switch text[i] {
		case '\\':
			if i+1 >= len(text) {
				lit.WriteByte('\\')
				i++
				continue
			}
			// Copy the escaped character whole so multi-byte runes survive.
			_, size := utf8.DecodeRuneInString(text[i+1:])
			lit.WriteString(text[i+1 : i+1+size])
			i += 1 + size
		case '[':
			end := strings.IndexByte(text[i:], ']')
			if end < 0 {
				return nil, lineErr(line, `unclosed "["; write \[ for a literal bracket`)
			}
			inner := text[i+1 : i+end]
			flush()
			if c, err := chord.Parse(inner); err == nil {
				toks = append(toks, Token{Kind: TokenChord, Chord: c})
			} else {
				toks = append(toks, Token{Kind: TokenAnnotation, Text: inner})
			}
			i += end + 1
		default:
			lit.WriteByte(text[i])
			i++
		}
	}
	flush()
	return toks, nil
}
