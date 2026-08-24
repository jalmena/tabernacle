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
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

type Options struct {
	// Transpose in semitones. Clamped and normalised by NormalizeTranspose.
	Transpose int
	Notation  chord.Notation
	Print     bool

	// Lang is the language the sheet's own words are in: the caption under a
	// diagram, the note explaining a tuning, the description a screen reader
	// reads out. The song is the writer's and is never touched.
	//
	// The zero value is the base language, so a caller that has no opinion still
	// gets sentences rather than keys.
	Lang i18n.Lang

	// Diagrams asks for guitar shapes alongside each chord. It is off by
	// default so that a listing page never pays for diagrams it does not show.
	Diagrams bool
	// Tuning is the song's tuning, used to decide which shapes are valid. An
	// unrecognised tuning yields no diagrams and an honest explanation rather
	// than shapes that are quietly wrong.
	Tuning voicing.Tuning

	// Simplify rewrites chords the collection has no shape for into ones it can
	// play. It needs Diagrams, since "playable" is a question about the shapes
	// we hold.
	Simplify bool
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
	Capo    int
	Genre   string
	Notes   []string // one entry per line, so the template emits a paragraph each

	Transpose int
	Notation  string

	// TuningNote explains an unrecognised tuning, so a reader is told why there
	// are no diagrams instead of assuming the feature is broken.
	TuningNote string

	// TabNote explains tablature that could not follow the chords, because a
	// fret cannot go below an open string and the neck runs out. Plainly stated,
	// with no alarm: it is information, not a problem the reader caused.
	TabNote string

	// Simplified is true when chords were rewritten into playable ones, and
	// SimplifyNote says so on the page — a sheet whose chords are not what the
	// file says must never keep that to itself.
	Simplified   bool
	SimplifyNote string
	// CanSimplify is true when offering to simplify would actually change
	// something. Most songs have nothing to simplify, so the control is hidden.
	CanSimplify bool

	// Shifted is true when the displayed chords are not what the file says,
	// which the page has to state plainly so nobody prints a sheet in the wrong
	// key without knowing.
	Shifted bool

	// Capos is advice, not a change: two or three places the capo could go so
	// the left hand holds easier shapes for the same sound. Empty when the sheet
	// is already as easy as it gets, which is most songs.
	Capos []CapoView

	Chords   []ChordRef
	Nav      []NavRef
	Sections []SectionView
}

// CapoView is one place to put the capo, ready to display.
type CapoView struct {
	Fret   int
	Where  string      // "Capo 3", or "No capo" at the nut
	InKey  string      // "play in D", in the reader's notation
	Chords []CapoChord // what the left hand holds there
	Cost   string      // "every chord open", "one barre"
}

// CapoChord is one chord in that advice. Text is for the eye and Query is the
// English spelling, because the chord page is addressed in English however the
// page is spelled.
type CapoChord struct {
	Text  string
	Query string
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
	Chord   string // "" when this cell carries no chord
	ChordID string
	// Title is what a reader sees on hovering the chord. It is built here and
	// not in the template because the template that draws a line is handed a
	// list of lines and nothing else — there is no language in reach down there.
	Title      string
	Annotation string // "" unless the cell is an annotation such as "x2"
	Text       string
}

// PrintedLines estimates how many lines of type a sheet takes.
//
// A chord line and the words under it are two lines of type, which is why a
// sheet with chords runs to roughly twice the length of its lyrics. Verbatim
// blocks count their own newlines, headings count one, and a blank line between
// stanzas counts one.
//
// It is an estimate and cannot be anything else: how many lines fit depends on
// the paper, the font and how far each line wraps. It is used for one decision —
// whether a song is long enough to be worth setting in two columns — where being
// a line or two out changes nothing.
func (v SheetView) PrintedLines() int {
	n := 0
	for _, sec := range v.Sections {
		if sec.Label != "" {
			n++
		}
		for i, b := range sec.Blocks {
			if i > 0 {
				n++ // the blank line between stanzas
			}
			if b.Verbatim {
				n += strings.Count(b.Text, "\n") + 1
				continue
			}
			for _, ln := range b.Lines {
				n++
				if ln.HasChord {
					n++
				}
			}
		}
	}
	return n
}

// Sheet renders a song.
func Sheet(s song.Song, o Options) SheetView {
	o.Transpose = NormalizeTranspose(o.Transpose)
	say := i18n.For(o.Lang)

	shift := o.Transpose
	shown := s
	var moved song.TransposeReport
	if shift != 0 {
		shown, moved = s.TransposeReport(shift)
	}

	v := SheetView{
		Title:     s.Title,
		Artist:    s.Artist,
		OrigKey:   formatSymbol(s.Key, o.Notation),
		Key:       formatSymbol(shown.Key, o.Notation),
		Tuning:    s.Tuning,
		Capo:      s.Capo,
		Genre:     s.Genre,
		Transpose: o.Transpose,
		Notation:  notationName(o.Notation),
		Shifted:   shift != 0,
	}
	if s.Notes != "" {
		v.Notes = strings.Split(s.Notes, "\n")
	}
	switch {
	case moved.TabsTooLow > 0:
		v.TabNote = say.T("tab.floor")
	case moved.TabsTooHigh > 0:
		v.TabNote = say.T("tab.ceiling")
	}
	if o.Diagrams && s.Tuning != "" {
		switch t, ok := voicing.ParseTuning(s.Tuning); {
		case !ok:
			v.TuningNote = say.T("tuning.unknown", s.Tuning)
		case !t.Equal(voicing.Standard()):
			// Said once for the sheet rather than on every chord, since in a
			// song like this it applies to nearly all of them.
			v.TuningNote = say.T("tuning.other", t.Name)
		}
	}

	vo := voicing.Options{Tuning: o.Tuning}
	if o.Diagrams {
		if o.Simplify {
			var moved int
			if shown, moved = simplify(shown, vo); moved > 0 {
				v.Simplified = true
				v.SimplifyNote = say.T("simplify.note")
			}
		} else {
			v.CanSimplify = wouldSimplify(shown, vo)
		}
	}

	for _, c := range shown.Chords() {
		ref := ChordRef{Text: c.Format(o.Notation), ID: ChordID(c)}
		if o.Diagrams {
			ref.Voicings, ref.Warning = chordVoicings(say, c, ref.Text, vo)
			// A card with no shape is a dead end unless it says where to go. The
			// sheet-wide control is the way out, so name it here rather than
			// leaving the reader to guess that the button at the top is related.
			if len(ref.Voicings) == 0 && v.CanSimplify {
				ref.Warning += " " + say.T("warn.trysimplify")
			}
		}
		v.Chords = append(v.Chords, ref)
	}

	if o.Diagrams {
		// The key decides the spelling of every suggestion, and a song that
		// declares none falls back to its first chord — the same guess a
		// guitarist makes when handed an unlabelled sheet, and the same one
		// song.Transpose makes.
		if key, ok := keyOf(shown); ok {
			v.Capos = capoAdvice(say, key, shown.Chords(), s.Capo, o.Notation, vo)
		}
	}

	anchors := make(map[string]int)
	for _, sec := range shown.Sections {
		sv := SectionView{
			Kind:   string(sec.Kind),
			Label:  sec.Label,
			Anchor: uniqueAnchor(anchors, sec),
			Blocks: blocksOf(say, sec, o.Notation),
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
func blocksOf(say *i18n.Printer, sec song.Section, no chord.Notation) []BlockView {
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
			current.Lines = append(current.Lines, lineOf(say, ln, no))
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
func lineOf(say *i18n.Printer, ln song.Line, no chord.Notation) LineView {
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
			out.Words = append(out.Words, WordView{Cells: alignToVowels(cells), Sep: sep})
			cells = nil
		}
	}

	for _, tok := range ln.Tokens {
		switch tok.Kind {
		case song.TokenChord:
			closeCell()
			cell.Chord = tok.Chord.Format(no)
			cell.ChordID = ChordID(tok.Chord)
			cell.Title = say.T("sheet.seeshape", cell.Chord)
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

// maxConsonantsMoved bounds how far a chord may slide within a word. Three
// covers the longest cluster that starts a syllable in Spanish or English
// ("str", "chr"), and stops a pathological run of letters from dragging a chord
// halfway across a word.
const maxConsonantsMoved = 3

// alignToVowels slides each chord onto the vowel of its syllable.
//
// A chord written as "A[G]mazing" lands the G on "mazing", so it prints above
// the "m". But the beat falls on the vowel — that is where a singer lands and
// where a player changes — so the G belongs above the "a". Moving the leading
// consonants back into the previous cell puts it there, and costs nothing: the
// text is unchanged, only the boundary between two cells moves.
//
// Nothing moves across a word boundary, so a chord already at the start of a
// word stays where it is.
func alignToVowels(cells []CellView) []CellView {
	for i := 1; i < len(cells); i++ {
		if cells[i].Chord == "" || cells[i].Annotation != "" || cells[i].Text == "" {
			continue
		}
		if cells[i-1].Annotation != "" {
			continue
		}
		lead := leadingConsonants(cells[i].Text)
		if lead == "" || lead == cells[i].Text {
			continue // already on a vowel, or nothing but consonants
		}
		cells[i-1].Text += lead
		cells[i].Text = cells[i].Text[len(lead):]
	}
	return cells
}

// leadingConsonants returns the run of letters before the syllable's vowel, or
// "" when the text already starts on one.
func leadingConsonants(text string) string {
	n, count := 0, 0
	for _, r := range text {
		if isVowel(r) {
			break
		}
		if !isLetter(r) {
			// Punctuation or a digit is not a consonant cluster; leave it be.
			return ""
		}
		count++
		if count > maxConsonantsMoved {
			return ""
		}
		n += len(string(r))
	}
	if n == len(text) {
		return "" // no vowel at all
	}
	return text[:n]
}

// isVowel covers the vowels a Spanish or English lyric actually uses, accents
// included. "y" is left out: at the start of a syllable it behaves as a
// consonant, which is exactly the position this is asking about.
func isVowel(r rune) bool {
	switch unicode.ToLower(r) {
	case 'a', 'e', 'i', 'o', 'u',
		'á', 'é', 'í', 'ó', 'ú', 'ü',
		'à', 'è', 'ì', 'ò', 'ù',
		'â', 'ê', 'î', 'ô', 'û',
		'ä', 'ë', 'ï', 'ö':
		return true
	}
	return false
}

func isLetter(r rune) bool { return unicode.IsLetter(r) }

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
