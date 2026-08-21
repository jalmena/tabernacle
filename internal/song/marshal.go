package song

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
)

// Marshal writes a song in canonical .song form.
//
// Marshal(Parse(x)) returns x byte for byte for any canonical file. That
// property is what makes it safe for the application to rewrite a file the
// user has been editing by hand: saving a song the user only transposed must
// not reflow the rest of the file.
//
// Two things are deliberately not preserved: comments in the front matter, and
// a format field older than the current one. The application should therefore
// only write a file back when something actually changed.
func Marshal(s Song) ([]byte, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(frontMatterDelim + "\n")
	writeField(&b, "title", quoteIfNeeded(s.Title))
	if s.Artist != "" {
		writeField(&b, "artist", quoteIfNeeded(s.Artist))
	}
	if s.Key != "" {
		writeField(&b, "key", quoteIfNeeded(s.Key))
	}
	if s.Capo != 0 {
		writeField(&b, "capo", strconv.Itoa(s.Capo))
	}
	if s.Tuning != "" {
		writeField(&b, "tuning", quoteIfNeeded(s.Tuning))
	}
	if s.Genre != "" {
		writeField(&b, "genre", quoteIfNeeded(s.Genre))
	}
	if s.Notes != "" {
		writeNotes(&b, s.Notes)
	}
	b.WriteString(frontMatterDelim + "\n")

	for _, sec := range s.Sections {
		b.WriteString("\n")
		if sec.Label != "" {
			b.WriteString("## " + sec.Label + "\n")
		}
		for _, ln := range sec.Lines {
			switch ln.Kind {
			case LineBlank:
				b.WriteString("\n")
			case LineVerbatim:
				fence := fenceFor(ln.Text)
				b.WriteString(fence + ln.Info + "\n")
				if ln.Text != "" {
					b.WriteString(ln.Text + "\n")
				}
				b.WriteString(fence + "\n")
			default:
				b.WriteString(formatLine(ln) + "\n")
			}
		}
	}
	return []byte(b.String()), nil
}

func writeField(b *strings.Builder, key, value string) {
	b.WriteString(key + ": " + value + "\n")
}

// writeNotes writes a multi-line note as a literal block and anything shorter
// as an ordinary value.
func writeNotes(b *strings.Builder, notes string) {
	if !strings.Contains(notes, "\n") {
		writeField(b, "notes", quoteIfNeeded(notes))
		return
	}
	b.WriteString("notes: |-\n")
	for _, l := range strings.Split(notes, "\n") {
		if l == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  " + l + "\n")
	}
}

// yamlKeywords are the plain words a YAML parser would read as something other
// than a string. Quoting them keeps a band called Yes a band called Yes.
var yamlKeywords = map[string]bool{
	"y": true, "n": true, "yes": true, "no": true, "true": true,
	"false": true, "on": true, "off": true, "null": true, "~": true,
}

// quoteIfNeeded quotes a value when leaving it bare would be ambiguous.
//
// The bar is set at what any YAML parser would do with the result, not at what
// this package's reader needs, so that the files stay portable.
func quoteIfNeeded(v string) string {
	if v == "" {
		return `""`
	}
	if yamlKeywords[strings.ToLower(v)] {
		return quote(v)
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return quote(v)
	}
	if strings.TrimSpace(v) != v {
		return quote(v)
	}
	if strings.ContainsAny(v, "\n\t\"'") || strings.Contains(v, ": ") || strings.Contains(v, " #") {
		return quote(v)
	}
	if strings.ContainsAny(v[:1], "&*!%@`{}[]|>,#-?:") {
		return quote(v)
	}
	return v
}

func quote(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(v[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// fenceFor returns a fence long enough to contain the block.
func fenceFor(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence
}

// formatLine reassembles a lyric line.
func formatLine(ln Line) string {
	var b strings.Builder
	for _, t := range ln.Tokens {
		switch t.Kind {
		case TokenChord:
			b.WriteString("[" + t.Chord.String() + "]")
		case TokenAnnotation:
			b.WriteString("[" + t.Text + "]")
		default:
			b.WriteString(escapeText(t.Text))
		}
	}
	out := b.String()
	// A lyric that happens to start like a heading or a fence needs an escape,
	// or reading the file back would turn it into one.
	if strings.HasPrefix(out, "#") || strings.HasPrefix(out, "```") {
		out = `\` + out
	}
	return out
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "[", `\[`)
}

// Validate reports whether a song can be written out and read back.
//
// Parse enforces the same rules with line numbers attached; this is the check
// for songs built in code, and the guard that stops Marshal from producing a
// file it could not itself read.
func Validate(s Song) error {
	if strings.TrimSpace(s.Title) == "" {
		return errors.New("title is required")
	}
	if strings.ContainsAny(s.Title, "\n") {
		return errors.New("title cannot span lines")
	}
	if err := validateKey(s.Key); err != nil {
		return err
	}
	if err := validateCapo(s.Capo); err != nil {
		return err
	}
	if err := validateGenre(s.Genre); err != nil {
		return err
	}
	for _, sec := range s.Sections {
		if strings.ContainsAny(sec.Label, "\n") {
			return fmt.Errorf("section label %q cannot span lines", sec.Label)
		}
		for _, ln := range sec.Lines {
			switch ln.Kind {
			case LineVerbatim:
				if strings.ContainsAny(ln.Info, "`\n") {
					return fmt.Errorf("fence tag %q cannot contain a backtick or a newline", ln.Info)
				}
			case LineLyric:
				for _, t := range ln.Tokens {
					if t.Kind == TokenAnnotation && strings.ContainsAny(t.Text, "]\n") {
						return fmt.Errorf("annotation %q cannot contain \"]\" or a newline", t.Text)
					}
					if t.Kind == TokenText && strings.Contains(t.Text, "\n") {
						return errors.New("a lyric line cannot contain a newline")
					}
				}
			}
		}
	}
	return nil
}

func validateKey(key string) error {
	if key == "" {
		return nil
	}
	if _, err := chord.Parse(key); err != nil {
		return fmt.Errorf("key %q is not a chord symbol such as G, Am or Bb", key)
	}
	return nil
}

// maxCapoFret is where a capo stops being a capo and starts being a mistake.
const maxCapoFret = 12

func validateCapo(capo int) error {
	if capo < 0 || capo > maxCapoFret {
		return fmt.Errorf("capo %d is out of range 0..%d", capo, maxCapoFret)
	}
	return nil
}

// validateGenre keeps a genre to one short label of plain text. It names a kind
// of music; it is not somewhere to put a paragraph.
func validateGenre(genre string) error {
	if genre == "" {
		return nil
	}
	if strings.ContainsAny(genre, "\n,") {
		return fmt.Errorf("genre %q must be a single label with no commas", genre)
	}
	if len(genre) > 60 {
		return fmt.Errorf("genre %q is longer than a label should be", genre)
	}
	return nil
}
