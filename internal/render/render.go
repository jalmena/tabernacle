// Package render turns a parsed song into the shape a template can print.
//
// It is pure: no HTTP, no SQL, no filesystem, and no error return, because
// there is nothing here that can fail. Every field it produces is a plain
// string, so html/template does all of the escaping in the correct context and
// no part of the application is ever in the business of building markup.
//
// Transposition, note names and the capo view are all presentation. None of
// them touches a file.
package render

import (
	"strings"
	"unicode"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// CapoView selects which pitches the chord symbols name.
type CapoView string

const (
	// CapoWritten shows the sheet exactly as written, which is what the file
	// says and what the player fingers.
	CapoWritten CapoView = ""
	// CapoSounding shows the pitches that actually sound with the capo on, which
	// is what to tell someone playing along without one.
	CapoSounding CapoView = "sounding"
)

// Options control how a song is rendered.
type Options struct {
	// Transpose in semitones. Clamped and normalised by NormalizeTranspose.
	Transpose int
	Notation  chord.Notation
	CapoView  CapoView
	Print     bool

	// Diagrams asks for guitar shapes alongside each chord. It is off by
	// default so that a listing page never pays for diagrams it does not show.
	Diagrams bool
	// Tuning is the song's tuning, used to decide which shapes are valid. An
	// unrecognised tuning yields no diagrams and an honest explanation rather
	// than shapes that are quietly wrong.
	Tuning voicing.Tuning
}

// MaxTranspose bounds the transposition controls. Beyond eleven semitones in
// either direction you have wrapped around to a spelling of the same key, so
// the extra range would only offer the reader worse ways to write it.
const MaxTranspose = 11

// NormalizeTranspose brings a requested transposition into range.
//
// A whole number of octaves is the same sheet, so it collapses to zero rather
// than showing a control that claims to have done something.
func NormalizeTranspose(n int) int {
	n %= 12
	if n > MaxTranspose {
		n -= 12
	}
	if n < -MaxTranspose {
		n += 12
	}
	return n
}

// SheetView is a song ready to be printed.
type SheetView struct {
	Title   string
	Artist  string
	Key     string // in the reader's notation, after transposition
	OrigKey string
	Tuning  string
	Meter   string
	Tempo   int
	Capo    int
	Tags    []string
	Notes   []string // one entry per line, so the template emits a paragraph each

	Transpose int
	Notation  string
	CapoView  CapoView

	// TuningNote explains an unrecognised tuning, so a reader is told why there
	// are no diagrams instead of assuming the feature is broken.
	TuningNote string

	// Shifted is true when the displayed chords are not what the file says,
	// which the page has to state plainly so nobody prints a sheet in the wrong
	// key without knowing.
	Shifted bool

	Chords   []ChordRef
	Nav      []NavRef
	Sections []SectionView
}

// ChordRef is one entry in the chords-used panel.
type ChordRef struct {
	Text string // as displayed, e.g. "F#m" or "Fa#m"
	ID   string // stable handle derived from the English spelling

	// Voicings is empty unless Options.Diagrams was set.
	Voicings []VoicingView
	// Warning explains an approximate or missing shape, ready to display.
	Warning string
}

// NavRef is a jump target for the section menu.
type NavRef struct {
	Label  string
	Anchor string
	Kind   string
}

// SectionView is one labelled part of a song.
type SectionView struct {
	Kind   string // song.Kind, usable directly as a CSS class
	Label  string // exactly as the user wrote it
	Anchor string
	Blocks []BlockView
}

// BlockView is a stanza or a verbatim block.
//
// Blocks exist because a blank line in a .song file is how the writer grouped
// the words, and a stanza must not be split across a page break when printed.
// A flat list of lines throws that grouping away and no stylesheet can recover
// it.
type BlockView struct {
	Verbatim bool
	Info     string // fence tag of a verbatim block, for example "tab"
	Text     string // verbatim content, newlines included
	Lines    []LineView
}

// LineView is one line of words with its chords.
type LineView struct {
	Words []WordView
	// HasChord lets the template leave out the chord row completely, so a verse
	// with no chords reads as tight plain text instead of text with an empty
	// row above every line.
	HasChord bool
}

// WordView is the unit that may wrap. Sep is the whitespace that followed it.
//
// Words exist because of "A[G]mazing": the chord lands inside the word, so two
// cells belong to one word. If cells were the wrap unit, a browser would
// cheerfully break "Amazing" in half and leave the chord stranded.
type WordView struct {
	Cells []CellView
	Sep   string
}

// CellView is one chord sitting above one run of text.
type CellView struct {
	Chord      string // "" when this cell carries no chord
	ChordID    string
	Annotation string // "" unless the cell is an annotation such as "x2"
	Text       string
}

// Sheet renders a song.
func Sheet(s song.Song, o Options) SheetView {
	o.Transpose = NormalizeTranspose(o.Transpose)

	shift := o.Transpose
	if o.CapoView == CapoSounding {
		// A capo raises everything it holds down, so the pitches that sound are
		// the written ones moved up by the capo's fret.
		shift += s.Capo
	}
	shown := s
	if shift != 0 {
		shown = s.Transpose(shift)
	}

	v := SheetView{
		Title:     s.Title,
		Artist:    s.Artist,
		OrigKey:   formatSymbol(s.Key, o.Notation),
		Key:       formatSymbol(shown.Key, o.Notation),
		Tuning:    s.Tuning,
		Meter:     s.Meter,
		Tempo:     s.Tempo,
		Capo:      s.Capo,
		Tags:      s.Tags,
		Transpose: o.Transpose,
		Notation:  notationName(o.Notation),
		CapoView:  o.CapoView,
		Shifted:   shift != 0,
	}
	if s.Notes != "" {
		v.Notes = strings.Split(s.Notes, "\n")
	}
	if o.Diagrams && s.Tuning != "" {
		switch t, ok := voicing.ParseTuning(s.Tuning); {
		case !ok:
			v.TuningNote = "No se conoce la afinación " + s.Tuning +
				", así que no se muestran posiciones de acordes."
		case !t.Equal(voicing.Standard()):
			// Said once for the sheet rather than on every chord, since in a
			// song like this it applies to nearly all of them.
			v.TuningNote = "Afinación " + t.Name +
				": se muestran solo las posiciones estándar que siguen siendo válidas."
		}
	}

	for _, c := range shown.Chords() {
		ref := ChordRef{Text: c.Format(o.Notation), ID: ChordID(c)}
		if o.Diagrams {
			ref.Voicings, ref.Warning = chordVoicings(c, ref.Text, voicing.Options{Tuning: o.Tuning})
		}
		v.Chords = append(v.Chords, ref)
	}

	anchors := make(map[string]int)
	for _, sec := range shown.Sections {
		sv := SectionView{
			Kind:   string(sec.Kind),
			Label:  sec.Label,
			Anchor: uniqueAnchor(anchors, sec),
			Blocks: blocksOf(sec, o.Notation),
		}
		v.Sections = append(v.Sections, sv)
		if sv.Label != "" {
			v.Nav = append(v.Nav, NavRef{Label: sv.Label, Anchor: sv.Anchor, Kind: sv.Kind})
		}
	}
	return v
}

// blocksOf groups a section's lines into stanzas, splitting on blank lines and
// breaking out verbatim blocks as their own units.
func blocksOf(sec song.Section, no chord.Notation) []BlockView {
	var (
		out     []BlockView
		current BlockView
	)
	flush := func() {
		if len(current.Lines) > 0 {
			out = append(out, current)
		}
		current = BlockView{}
	}
	for _, ln := range sec.Lines {
		switch ln.Kind {
		case song.LineBlank:
			flush()
		case song.LineVerbatim:
			flush()
			out = append(out, BlockView{Verbatim: true, Info: ln.Info, Text: ln.Text})
		default:
			current.Lines = append(current.Lines, lineOf(ln, no))
		}
	}
	flush()
	return out
}

// lineOf reshapes a token run into words and cells.
//
// The token stream carries a chord's position implicitly, by where it sits
// between runs of text: "A[G]mazing" arrives as text "A", chord G, text
// "mazing". Splitting on whitespace and grouping into words is what lets the
// page reflow on a phone without ever separating a chord from its syllable.
func lineOf(ln song.Line, no chord.Notation) LineView {
	var (
		out   LineView
		cells []CellView
		cell  CellView
	)

	closeCell := func() {
		if cell != (CellView{}) {
			cells = append(cells, cell)
			cell = CellView{}
		}
	}
	closeWord := func(sep string) {
		closeCell()
		// A word with no cells but a separator is meaningful: it carries the
		// indentation or spacing that was in the file, and keeping it makes the
		// rendering an exact reconstruction of the original line.
		if len(cells) > 0 || sep != "" {
			out.Words = append(out.Words, WordView{Cells: cells, Sep: sep})
			cells = nil
		}
	}

	for _, tok := range ln.Tokens {
		switch tok.Kind {
		case song.TokenChord:
			closeCell()
			cell.Chord = tok.Chord.Format(no)
			cell.ChordID = ChordID(tok.Chord)
			out.HasChord = true
		case song.TokenAnnotation:
			closeCell()
			// An annotation is its own cell and never a chord, so "[x2]" is
			// never transposed and never offered as a shape to learn.
			cells = append(cells, CellView{Annotation: tok.Text})
		default:
			for _, part := range splitKeepingSpaces(tok.Text) {
				if isSpace(part) {
					closeWord(part)
					continue
				}
				cell.Text += part
			}
		}
	}
	closeWord("")
	return out
}

// splitKeepingSpaces breaks text into alternating runs of whitespace and
// non-whitespace, keeping both.
func splitKeepingSpaces(s string) []string {
	var (
		out   []string
		start int
		inSp  bool
	)
	for i, r := range s {
		sp := unicode.IsSpace(r)
		if i == 0 {
			inSp = sp
			continue
		}
		if sp != inSp {
			out = append(out, s[start:i])
			start = i
			inSp = sp
		}
	}
	if s != "" {
		out = append(out, s[start:])
	}
	return out
}

func isSpace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return s != ""
}

// ChordID is a stable handle for a chord, safe in an id attribute and in a URL.
//
// It is built from the canonical English spelling, so the same chord gets the
// same handle whether the reader is looking at "C#m7" or "Do#m7" — the display
// spelling is a presentation choice and must not change which shape a click
// opens. The sharp and flat signs are spelled out because "#" starts a URL
// fragment and "/" is a path separator, and both appear in ordinary chord
// symbols.
func ChordID(c chord.Chord) string {
	var b strings.Builder
	for _, r := range c.String() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r == '#':
			// A flat needs no escape: it is already the letter "b", which the
			// lowercase case above has handled.
			b.WriteString("s")
		case r == '/':
			b.WriteString("_")
		case r == '(' || r == ')' || r == '+' || r == '-':
			b.WriteString("-")
		default:
			b.WriteString("-")
		}
	}
	return strings.Trim(b.String(), "-")
}

// formatSymbol re-spells a stored chord symbol in the reader's notation,
// leaving anything that is not a chord alone.
func formatSymbol(sym string, no chord.Notation) string {
	if sym == "" {
		return ""
	}
	c, err := chord.Parse(sym)
	if err != nil {
		return sym
	}
	return c.Format(no)
}

func notationName(no chord.Notation) string {
	if no == chord.Solfege {
		return "solfege"
	}
	return "english"
}

// uniqueAnchor builds a jump target for a section, keeping it unique within the
// sheet so two verses do not share an anchor.
func uniqueAnchor(seen map[string]int, sec song.Section) string {
	base := slugifyLabel(sec.Label)
	if base == "" {
		base = string(sec.Kind)
	}
	if base == "" {
		base = "section"
	}
	seen[base]++
	if n := seen[base]; n > 1 {
		return base + "-" + itoa(n)
	}
	return base
}

// slugifyLabel reduces a heading to the restricted alphabet an anchor may use.
func slugifyLabel(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
