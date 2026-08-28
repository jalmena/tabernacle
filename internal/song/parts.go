package song

import (
	"strconv"
	"strings"
)

// Taking a song apart so it can be edited a field and a section at a time, and
// putting it back together again.
//
// The body is not re-serialised. Each section keeps the exact text that was
// written in it, and putting the song back together is the metadata block
// followed by those texts joined up. So editing one section cannot disturb
// another, and a section nobody touches comes back byte for byte — which
// Marshal alone cannot promise, since it rebuilds every line from the tokens the
// parser made of it.
//
// The metadata block is a different matter: it is a small set of named fields,
// and Front writes it the one canonical way.

// Parts is a song taken apart for editing.
type Parts struct {
	Title  string
	Artist string
	Key    string
	Capo   string // a string, so that a half-typed number survives a redraw
	Tuning string
	Genre  string
	Notes  string

	Sections []Part
}

// Part is one labelled section, with its lines exactly as they were written.
type Part struct {
	Label string
	Body  string
}

// Take pulls a song apart. It reports the error rather than guessing when the
// text is not a song file at all.
//
// A song with no title yet is taken apart happily: an editor holds a half
// written song most of the time, and refusing to show one as fields until it had
// a title meant somebody who looked at the file and came back was told to name
// their song before they could carry on typing it.
func Take(text []byte) (Parts, error) {
	s, err := Draft(text)
	if err != nil {
		return Parts{}, err
	}
	p := Parts{
		Title: s.Title, Artist: s.Artist, Key: s.Key,
		Tuning: s.Tuning, Genre: s.Genre, Notes: s.Notes,
	}
	if s.Capo != 0 {
		p.Capo = strconv.Itoa(s.Capo)
	}
	p.Sections = cutBody(body(text))
	return p, nil
}

// body is everything after the metadata block, which is what the sections are
// cut out of.
//
// Where the block ends is the parser's question and it answers it here too, by
// being asked. Working it out again separately is how a closing delimiter with a
// trailing space came to end the block for one of them and not the other, and
// the body went missing.
func body(text []byte) string {
	lines := strings.Split(newlines(string(text)), "\n")
	n, err := frontMatterLines(lines)
	if err != nil {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[n:], "\n")
}

// cutBody splits a body into its labelled sections.
//
// It walks the lines the way the parser walks them, fences and all. A heading
// inside a fenced block is tablature and not a heading: a solo whose grid
// happened to contain a line of hashes came apart in the middle, and half of it
// turned into a section of its own.
//
// Anything before the first heading is a section with no label, which is what
// the parser makes of it and what a one-verse song written without headings is.
func cutBody(text string) []Part {
	lines := strings.Split(text, "\n")
	var parts []Part
	var label string
	var kept []string
	flush := func() {
		joined := strings.Trim(strings.Join(kept, "\n"), "\n")
		if label != "" || joined != "" {
			parts = append(parts, Part{Label: label, Body: joined})
		}
		label, kept = "", nil
	}

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		if strings.HasPrefix(raw, "```") {
			marker := len(raw) - len(strings.TrimLeft(raw, "`"))
			kept = append(kept, raw)
			for i++; i < len(lines); i++ {
				kept = append(kept, lines[i])
				if isFenceClose(strings.TrimRight(lines[i], " \t"), marker) {
					break
				}
			}
			continue
		}
		if strings.HasPrefix(raw, "## ") {
			flush()
			label = strings.TrimSpace(raw[3:])
			continue
		}
		kept = append(kept, raw)
	}
	flush()
	return parts
}

// Put joins a song back into a file.
//
// It does not validate. A song being edited is half written most of the time,
// and the place to be told about that is when it is saved, with a line number
// attached, which is what Parse does.
func Put(p Parts) []byte {
	capo, _ := strconv.Atoi(strings.TrimSpace(p.Capo))
	var b strings.Builder
	// Nothing is trimmed here. Tidying what somebody typed is the form's job and
	// it does it before the value ever reaches this; doing it again here would
	// mean a song could not be taken apart and put back unchanged, and a title
	// that really does end in a space would quietly lose it.
	b.WriteString(Front(Song{
		Title: p.Title, Artist: p.Artist,
		Key: p.Key, Capo: capo,
		Tuning: p.Tuning, Genre: p.Genre,
		// Not trimmed. A literal block may begin or end with a blank line and
		// that is content, not slack; how to write it back is a question Front
		// already answers, and answering it twice is how it got changed.
		Notes: p.Notes,
	}))
	for _, sec := range p.Sections {
		if sec.Label == "" && strings.TrimSpace(sec.Body) == "" {
			continue
		}
		b.WriteString("\n")
		if sec.Label != "" {
			b.WriteString("## " + sec.Label + "\n")
		}
		if body := strings.Trim(newlines(sec.Body), "\n"); body != "" {
			b.WriteString(body + "\n")
		}
	}
	return []byte(b.String())
}
