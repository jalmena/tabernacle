package song

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Reading a chord sheet: the chords on a line of their own above the words they
// fall on, which is how most songs on the web and in notebooks are written down.
//
// The file format is not built on that layout, because reading it is guesswork:
// a short line of words can look like a line of chords, and a chord's column is
// only as right as the font it was typed in. So it is read here, at the door,
// and nowhere else. What comes out is ordinary sections with the chords in
// brackets, and the person bringing the song in sees it before it matters — in
// the editor's fields, or in the import report and the song it links to.

// IsFile reports whether text is a .song file rather than a chord sheet, by the
// line every .song file starts with.
func IsFile(text []byte) bool {
	first, _, _ := strings.Cut(newlines(strings.TrimPrefix(string(text), "\ufeff")), "\n")
	return strings.TrimRight(first, " \t") == frontMatterDelim
}

// FromSheet reads a chord sheet into the parts of a song.
//
// Title and artist are the caller's to fill in. A sheet does not say them in
// any way that can be relied on, and the caller usually knows them from
// somewhere better: the name of the file, or a field the reader typed into.
//
// verse is what to call a stanza of words that follows a named section without
// a name of its own, as the second verse after a chorus usually does on a sheet.
// In a song file a section runs to the next heading, so left unnamed it would be
// read as more of the chorus.
//
// A sheet typed entirely in capitals has its words brought down to ordinary
// sentence case, since nothing else on the page is shouting.
func FromSheet(text, verse string) Parts {
	lines := strings.Split(newlines(strings.TrimPrefix(text, "\ufeff")), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(expandTabs(l), unicode.IsSpace)
	}

	var p Parts
	var cur Part
	var kept []string
	// named is true while the section is one the sheet named, a line such as
	// "Estribillo:", and has not yet given way to a verse.
	var named bool
	flush := func() {
		cur.Body = strings.Trim(strings.Join(kept, "\n"), "\n")
		if cur.Label != "" || cur.Body != "" {
			p.Sections = append(p.Sections, cur)
		}
		cur, kept = Part{}, nil
	}
	// One blank line between stanzas, however many the sheet had.
	keep := func(l string) {
		if l == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			return
		}
		kept = append(kept, l)
	}
	// Words after a blank line in a section the sheet named are where that
	// section ends, because sheets name the chorus and leave the verses unnamed.
	// Only the first time: a sheet with a blank line between every line would
	// otherwise come apart into a section per line, so what follows stays
	// together until the sheet names something again.
	stanza := func() {
		if named && len(kept) > 0 && kept[len(kept)-1] == "" {
			flush()
			cur.Label, named = verse, false
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "```"):
			// Already in the format, so copied whole, blank lines and all.
			marker := len(line) - len(strings.TrimLeft(line, "`"))
			kept = append(kept, line)
			for i++; i < len(lines); i++ {
				kept = append(kept, lines[i])
				if isFenceClose(lines[i], marker) {
					break
				}
			}
		case strings.HasPrefix(line, "## "):
			// A heading in the format itself, whose writer knew it runs to
			// the next one.
			flush()
			cur.Label, named = strings.TrimSpace(line[3:]), false
		case isTabRow(line) && (tabRun(lines[i:]) >= 3 || strings.Contains(line, "|")):
			n := tabRun(lines[i:])
			kept = append(kept, "```tab")
			for _, row := range lines[i : i+n] {
				kept = append(kept, strings.TrimSpace(row))
			}
			kept = append(kept, "```")
			i += n - 1
		case isRule(line):
			// A row of dashes drawn between parts is a gap, not words.
			keep("")
		default:
			if n, ok := capoLine(line); ok && p.Capo == "" {
				p.Capo = strconv.Itoa(n)
				continue
			}
			if label, rest, ok := sectionLine(line); ok {
				flush()
				cur.Label, named = label, true
				if cs, ok := chordLine(rest); ok {
					keep(alone(cs))
				} else if rest != "" {
					keep(words(rest))
				}
				continue
			}
			if cs, ok := chordLine(line); ok {
				if i+1 < len(lines) && isLyric(lines[i+1]) {
					stanza()
					keep(merge(cs, lines[i+1]))
					i++
				} else {
					keep(alone(cs))
				}
				continue
			}
			if line != "" {
				stanza()
			}
			keep(words(line))
		}
	}
	flush()

	if shouting(p.Sections) {
		for i := range p.Sections {
			p.Sections[i].Body = calm(p.Sections[i].Body)
		}
	}
	return p
}

// shouting reports whether the words of a song are written in capitals: nearly
// every letter outside the chords and the tablature, and enough of them to be
// sure it is the whole song and not an acronym.
func shouting(sections []Part) bool {
	upper, lower := 0, 0
	for _, s := range sections {
		eachLine(s.Body, func(line string) string {
			inChord := false
			for _, r := range line {
				switch {
				case r == '[':
					inChord = true
				case r == ']':
					inChord = false
				case inChord:
				case unicode.IsUpper(r):
					upper++
				case unicode.IsLower(r):
					lower++
				}
			}
			return line
		})
	}
	return upper >= 20 && lower*10 <= upper
}

// calm puts the words of a section into sentence case, leaving the chords and
// any fenced block exactly as they are.
func calm(body string) string {
	return eachLine(body, sentenceCase)
}

// eachLine applies fn to every line of a body that is words, which is every
// line outside a fenced block.
func eachLine(body string, fn func(string) string) string {
	lines := strings.Split(body, "\n")
	fence := 0
	for i, l := range lines {
		switch {
		case fence > 0:
			if isFenceClose(l, fence) {
				fence = 0
			}
		case strings.HasPrefix(l, "```"):
			fence = len(l) - len(strings.TrimLeft(l, "`"))
		default:
			lines[i] = fn(l)
		}
	}
	return strings.Join(lines, "\n")
}

// sentenceCase lowers a line and raises the letter that begins each sentence in
// it: the first one, and any after a full stop, a question mark or an
// exclamation mark, the Spanish ones that open a sentence included. Names lose
// their capital, which is the price of not being able to tell one from a word.
func sentenceCase(line string) string {
	var b strings.Builder
	start, inChord, escaped := true, false, false
	for _, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case inChord:
			inChord = r != ']'
		case r == '[':
			inChord = true
		case unicode.IsLetter(r):
			if start {
				r, start = unicode.ToUpper(r), false
			} else {
				r = unicode.ToLower(r)
			}
		case strings.ContainsRune(".!?¡¿", r):
			start = true
		}
		b.WriteRune(r)
	}
	return b.String()
}

// placed is a chord, or a mark that travels with chords, and the column of the
// sheet it starts at.
type placed struct {
	col  int
	text string
}

var (
	// marks are what a sheet writes among its chords that is not a chord: a
	// repeat, "bis", "no chord". They stay, as annotations.
	marks = regexp.MustCompile(`^\(?(?:[xX] ?\d{1,2}|\d{1,2}[xX]|[bB][iI][sS]|N\.?C\.?)\)?$`)
	// bars are rhythm strokes and bar lines, which say nothing a reader of the
	// page needs and are dropped.
	bars = regexp.MustCompile(`^[|/\\%:.\-]+$`)
	// repeats is a repeat written out in words, "(2 veces)", which a sheet puts
	// at the end of a line of chords.
	repeats = regexp.MustCompile(`(?i)\(?\b(\d{1,2})\s*(?:veces|vegades|times|fois|volte|mal)\b\)?`)
)

// chordLine reads a line made only of chords into the chords and where each one
// starts. Anything that is neither a chord nor a mark that goes with them makes
// it a line of words, and then it reports false.
//
// Solfege is read as well as letter names, and written back as letter names,
// because that is what a song file stores.
func chordLine(line string) ([]placed, bool) {
	line = repeats.ReplaceAllStringFunc(line, func(m string) string {
		// Written as a mark, and padded to the same width so that nothing
		// after it moves.
		mark := "x" + repeats.FindStringSubmatch(m)[1]
		return mark + strings.Repeat(" ", max(utf8.RuneCountInString(m)-len(mark), 0))
	})
	var out []placed
	chords := 0
	for _, f := range fieldsAt(line) {
		switch {
		case bars.MatchString(f.text):
		case marks.MatchString(f.text):
			out = append(out, placed{f.col, strings.Trim(f.text, "()")})
		default:
			// A chord a sheet trails off after, "LAm.....", is still the chord.
			sym := strings.TrimRight(f.text, ".,;")
			c, err := chord.Parse(sym)
			if err != nil {
				if c, err = chord.ParseIn(chord.Solfege, solfegeCase(sym)); err != nil {
					return nil, false
				}
			}
			out = append(out, placed{f.col, c.Format(chord.English)})
			chords++
		}
	}
	return out, chords > 0
}

// shoutedSyllable is a solfege note name in capitals, as sheets often write
// them: LAm, MI7, SOL/SI.
var shoutedSyllable = regexp.MustCompile(`(^|/)(SOL|DO|RE|MI|FA|LA|SI)`)

// solfegeCase writes a solfege note name the way the chord reader expects it,
// La rather than LA, and leaves everything else alone.
func solfegeCase(s string) string {
	return shoutedSyllable.ReplaceAllStringFunc(s, func(m string) string {
		slash := strings.HasPrefix(m, "/")
		name := strings.TrimPrefix(m, "/")
		name = name[:1] + strings.ToLower(name[1:])
		if slash {
			return "/" + name
		}
		return name
	})
}

// fieldsAt splits a line at its spaces, keeping the column each piece starts at.
// Columns are counted in characters rather than bytes, or every accented letter
// in front of a chord would push it one place to the right.
func fieldsAt(line string) []placed {
	var out []placed
	col, start := 0, -1
	var b strings.Builder
	for _, r := range line {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, placed{start, b.String()})
				start = -1
				b.Reset()
			}
		} else {
			if start < 0 {
				start = col
			}
			b.WriteRune(r)
		}
		col++
	}
	if start >= 0 {
		out = append(out, placed{start, b.String()})
	}
	return out
}

// merge puts each chord into the line of words below it, at the column it stood
// over. A chord that stood over the gap between two words belongs to the word
// after it, since chords are typed a column early far more often than late, and
// one that stood past the end of the words comes after them.
func merge(cs []placed, line string) string {
	w := []rune(line)
	indent := 0
	for indent < len(w) && unicode.IsSpace(w[indent]) {
		indent++
	}
	w = w[indent:]

	var b strings.Builder
	at := 0
	for _, c := range cs {
		col := max(c.col-indent, 0)
		for col < len(w) && unicode.IsSpace(w[col]) {
			col++
		}
		if col > len(w) {
			b.WriteString(string(w[at:]) + " ")
			at = len(w)
		} else {
			b.WriteString(string(w[at:col]))
			at = col
		}
		b.WriteString("[" + c.text + "]")
	}
	b.WriteString(string(w[at:]))
	return escapeStart(b.String())
}

// alone writes a line of chords with no words under it, an intro or a turnaround.
func alone(cs []placed) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = "[" + c.text + "]"
	}
	return strings.Join(parts, " ")
}

// words is a line that holds no chords, as a lyric line of a song file.
func words(line string) string { return escapeStart(strings.TrimSpace(line)) }

// escapeStart keeps a line of words from being read as a heading, a reserved
// line or a fence, the same way the writer does.
func escapeStart(line string) string {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
		return `\` + line
	}
	return line
}

// isLyric reports whether a line is words that a line of chords above it could
// belong to.
func isLyric(line string) bool {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "```") ||
		strings.HasPrefix(line, "## ") || isTabRow(line) || isRule(line) {
		return false
	}
	if _, ok := chordLine(line); ok {
		return false
	}
	if _, _, ok := sectionLine(line); ok {
		return false
	}
	_, ok := capoLine(line)
	return !ok
}

// sectionLine recognises a line that names a part of the song — "Estribillo",
// "CORO:", "[Chorus]", "----estribillo----", "Intro: Am G" — by the same names a
// heading is understood by, and returns the name and whatever followed a colon.
func sectionLine(line string) (label, rest string, ok bool) {
	name := strings.TrimSpace(line)
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name, rest = name[:i], strings.TrimSpace(name[i+1:])
	}
	// What a sheet draws around a name is decoration, not part of it.
	name = strings.Trim(name, " \t-=_~*·.#[](){}<>")
	if name == "" || utf8.RuneCountInString(name) > 24 || KindForLabel(name) == KindOther {
		return "", "", false
	}
	// All in capitals is a sheet shouting, and all in small letters is one
	// that did not bother; either way the name is written as a name.
	if name == strings.ToUpper(name) || name == strings.ToLower(name) {
		r, n := utf8.DecodeRuneInString(name)
		name = string(unicode.ToUpper(r)) + strings.ToLower(name[n:])
	}
	return name, rest, true
}

// capoWords finds the fret on a line such as "Cejilla en 2" or "Capo 3rd fret".
var capoWords = regexp.MustCompile(`(?i)^\s*(?:capo|cejilla|capodastro|capotasto)\b\D{0,12}?(\d{1,2})(?:\D|$)`)

// capoLine reads a line that says where the capo goes. It has to be short, so
// that a lyric which happens to start with the word is left alone.
func capoLine(line string) (int, bool) {
	if utf8.RuneCountInString(strings.TrimSpace(line)) > 30 {
		return 0, false
	}
	m := capoWords.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	return n, validateCapo(n) == nil && n > 0
}

// tabRow is one string of a tablature: an optional string name and bar, then
// frets, dashes and the letters that mark how a note is played. Plenty of
// sheets write neither the name nor the bar, only "-6-6-5-3--".
var tabRow = regexp.MustCompile(`^\s*(?:[A-Ga-g][#b]?\s*)?[|:]?[-0-9hpbrsxtv/\\~^().*<>|= ]*$`)

func isTabRow(line string) bool {
	return tabRow.MatchString(line) && strings.Count(line, "-") >= 3
}

// tabRun counts the rows of tablature at the start of lines. A run of three or
// more is a tablature even without bars, since one string on its own is far
// more likely to be a row of dashes drawn between two parts.
func tabRun(lines []string) int {
	n := 0
	for n < len(lines) && isTabRow(lines[n]) {
		n++
	}
	return n
}

// rule matches a line drawn to separate parts of a sheet: dashes, equals signs
// and the like, and nothing else.
var rule = regexp.MustCompile(`^\s*[-=_~*·.]{3,}[-=_~*·.\s]*$`)

func isRule(line string) bool { return rule.MatchString(line) }

// expandTabs turns tab characters into the spaces they stood for, so that a
// chord's column means the same thing on both lines.
func expandTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
