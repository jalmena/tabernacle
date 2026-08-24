package render

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// LabOptions is what the chord page needs in order to answer.
type LabOptions struct {
	Notation chord.Notation
	Lang     i18n.Lang
	Start    int    // first fret of the editable window
	Grip     string // the shape drawn on the board
	Symbol   string // the chord asked for, as written
	All      bool
}

// FretCell is one place a finger can go.
//
// Top and Nut are about the line drawn above the cell, which only the first fret
// of the window has: the nut when the window starts at the first fret, an
// ordinary fret line when it starts anywhere else.
type FretCell struct {
	Fret int
	On   bool
	Top  bool
	Nut  bool
}

// StringRow is one string of the editable neck.
//
// Muted and Open are separate from the frets because they are separate choices a
// player makes, not fret zero and fret minus one.
type StringRow struct {
	Index int    // 0 is the lowest string, which is how shapes are written
	Name  string // "6ª"
	Muted bool
	Open  bool
	Cells []FretCell
	// First and Last mark the outside strings, which draw half a fret line each
	// so that the grid ends on the sixth and the first rather than half a column
	// past them.
	First bool
	Last  bool
}

// FretboardView is the editable neck.
//
// Held upright, strings running down the page with the sixth on the left, which
// is how a chord chart is drawn and therefore how the diagrams beside it are
// drawn. Five frets, the same five a diagram shows, so a shape can be copied
// from a picture onto the board fret for fret.
//
// It is a form control and not a drawing: the choices are radio buttons, so the
// neck can be played with by keyboard, read aloud, and used with no JavaScript
// at all.
type FretboardView struct {
	Start   int
	Strings []StringRow
	Grip    string
	Down    string // where the window slides, empty at the end of the neck
	Up      string

	// Pitches are the notes this shape sounds, as MIDI numbers, lowest string
	// first and comma separated. The browser needs them to make a sound and has
	// no business working them out: which note a fret on a string produces is
	// music, and music is decided in Go where it is tested.
	Pitches string
}

// ReadingView is one name for the notes drawn on the board.
type ReadingView struct {
	Name  string
	Value string // the symbol as this package writes it, for a link to ask for
	URL   string
	Note  string // "sin la quinta", "con Mi en el bajo"
	On    bool   // the reading the rest of the page is about
}

// LabShape is one diagram in the list of ways to hold a chord.
type LabShape struct {
	Shape VoicingView
	Note  string // where on the neck it sits, and whether it is barred
	Grip  string
	Start int // the window the neck should open on to show it whole
	URL   string
	On    bool // this is the shape currently on the neck
}

// SuggestionView is a chord the drawn shape would play if some of its ringing
// strings were stopped, together with the shape that would do it.
//
// Following one applies the stopping as well as naming the chord, because seeing
// the neck turn into the real shape is the part that teaches.
type SuggestionView struct {
	Name  string
	Value string
	Grip  string
	Start int
	Note  string // "sin la 6ª y la 5ª"
	URL   string
}

// InversionView points at the same chord over a different bass, which is a
// different chord and gets its own page rather than being piled onto this one.
type InversionView struct {
	Name  string
	Value string // the symbol as this package writes it, for a link to ask for
	URL   string
	Total int
}

// LabView is the whole chord page.
type LabView struct {
	Symbol   string // what was asked for, as written
	Chord    string // what it was understood as, formatted
	Problem  string // an honest note when the input made no sense
	Board    FretboardView
	Readings []ReadingView
	Sounds   []string // the notes drawn, when none of them make a chord
	// Nearly are the chords the shape would play with a ringing string or two
	// stopped. Somebody learning does not reliably hear the difference between a
	// string they stopped and one they left open.
	Nearly []SuggestionView
	// Shapes is every way of holding the chord, easiest first and undivided:
	// somebody learning a chord wants the one they can play today and then the
	// next one, which is an order rather than a set of categories.
	Shapes     []LabShape
	Total      int
	Inversions []InversionView
	All        bool
	AllURL     string
	Roots      []Choice
	Qualities  []Choice
}

// Choice is one option in the chord chooser.
type Choice struct {
	Label string
	Value string
	On    bool
}

// labFrets is how many frets the editable neck shows.
//
// The same five the diagrams draw, so that a shape read off a picture can be
// copied onto the board fret for fret. It is not the four the generator searches
// in: that number is about how far a hand stretches, this one is about what a
// reader can see at once.
const labFrets = 5

// maxStart keeps the window on the neck. Twelve is where the octave repeats, and
// the generator does not look past it either.
const maxStart = 12

// Lab answers the chord page: what the reader drew, and every way to play what
// they asked for.
//
// The two halves feed each other. Naming a chord puts its easiest shape on the
// neck, because a reader who asks for F wants to see F and not an empty grid;
// drawing on the neck names what the notes are. An empty page starts with the
// six strings ringing open, which is what a guitar does when nobody is holding
// it.
func Lab(o LabOptions) LabView {
	say := i18n.For(o.Lang)
	v := LabView{Symbol: o.Symbol, All: o.All}
	v.Roots, v.Qualities = chooserFor(say, o)
	v.Shapes, v.Total, v.Inversions, v.Chord, v.Problem = variantsFor(say, o)

	frets, ok := voicing.ParseGrip(o.Grip)
	if !ok || len(frets) != voicing.Standard().Strings() {
		frets = nil
	}
	start := o.Start
	switch {
	case frets != nil:
	case len(v.Shapes) > 0:
		frets, _ = voicing.ParseGrip(v.Shapes[0].Grip)
		if start == 0 {
			start = v.Shapes[0].Start
		}
	default:
		frets = make([]voicing.Fret, voicing.Standard().Strings())
	}
	if start < 1 {
		start = 1
	}
	if start > maxStart {
		start = maxStart
	}

	drawn := voicing.Voicing{Tuning: voicing.Standard(), Frets: frets}
	v.Board = boardFor(say, drawn, start, o)

	for _, r := range voicing.Identify(frets, voicing.Standard()) {
		sym := r.Chord.Format(chord.English)
		v.Readings = append(v.Readings, ReadingView{
			Name:  r.Chord.Format(o.Notation),
			Value: sym,
			URL:   labURL(sym, drawn.Grip(), start, o.All),
			Note:  readingNote(say, r, o.Notation),
		})
	}
	if len(v.Readings) == 0 {
		for _, pc := range drawn.PitchClasses() {
			v.Sounds = append(v.Sounds, chord.Spell(pc, chord.Sharps).Format(o.Notation))
		}
	}

	// An untouched neck has nothing to nearly play, so it is not offered six
	// strings' worth of guesses before the reader has done anything.
	if drawn.Reach() > 0 {
		for _, sg := range voicing.Suggest(frets, voicing.Standard()) {
			if len(v.Nearly) == howManyNearly {
				break
			}
			quiet := slices.Clone(frets)
			for _, i := range sg.Silence {
				quiet[i] = voicing.Muted
			}
			hushed := voicing.Voicing{Tuning: voicing.Standard(), Frets: quiet}
			sym := sg.Chord.Format(chord.English)
			v.Nearly = append(v.Nearly, SuggestionView{
				Name:  sg.Chord.Format(o.Notation),
				Value: sym,
				Grip:  hushed.Grip(),
				Start: start,
				Note:  withoutStrings(say, sg.Silence),
				URL:   labURL(sym, hushed.Grip(), start, o.All),
			})
		}
	}

	// The reading the page is currently about is the one the shapes below belong
	// to, and it says so. Failing that, the first: it is the likeliest name for
	// what is drawn, which is a thing worth pointing at even before anybody has
	// chosen.
	if c, ok := parseSymbol(o); ok {
		want := c.Format(chord.English)
		for i := range v.Readings {
			v.Readings[i].On = v.Readings[i].Value == want
		}
	}
	if len(v.Readings) > 0 && !slices.ContainsFunc(v.Readings, func(r ReadingView) bool { return r.On }) {
		v.Readings[0].On = true
	}

	here := drawn.Grip()
	for i := range v.Shapes {
		v.Shapes[i].On = v.Shapes[i].Grip == here
	}
	if v.Total > len(v.Shapes) {
		v.AllURL = labURL(o.Symbol, here, start, true)
	}
	return v
}

// boardFor lays out the editable neck with the drawn shape on it.
func boardFor(say *i18n.Printer, v voicing.Voicing, start int, o LabOptions) FretboardView {
	b := FretboardView{Start: start, Grip: v.Grip(), Pitches: midiList(v.Pitches())}
	if start > 1 {
		b.Down = labURLAt(o, v.Grip(), start-1)
	}
	if start < maxStart {
		b.Up = labURLAt(o, v.Grip(), start+1)
	}
	for i, f := range v.Frets {
		row := StringRow{
			Index: i,
			Muted: f == voicing.Muted,
			Open:  f == voicing.Open,
			First: i == 0,
			Last:  i == len(v.Frets)-1,
		}
		if len(v.Frets) == 6 {
			row.Name = say.T(stringKey(i, "short"))
		}
		for n := start; n < start+labFrets; n++ {
			row.Cells = append(row.Cells, FretCell{
				Fret: n,
				On:   int(f) == n,
				Top:  n == start && start > 1,
				Nut:  n == start && start == 1,
			})
		}
		b.Strings = append(b.Strings, row)
	}
	return b
}

// howManyNearly is how many near misses are worth offering. More than a handful
// stops being a hint and becomes a list of everything the six strings could
// almost be.
const howManyNearly = 4

// withoutStrings names the strings a suggestion asks to be stopped.
func withoutStrings(say *i18n.Printer, at []int) string {
	parts := make([]string, 0, len(at))
	for _, i := range at {
		if i >= 0 && i < 6 {
			parts = append(parts, say.T(stringKey(i, "the")))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return say.T("nearly.without", joinWords(say, parts))
}

// readingNote says what is unusual about a reading, so that "Do7 sin la quinta"
// is not silently presented as an ordinary Do7.
func readingNote(say *i18n.Printer, r voicing.Reading, no chord.Notation) string {
	var parts []string
	for _, iv := range r.Omitted {
		if key, ok := intervalKeys[iv]; ok {
			parts = append(parts, say.T("reading.without", say.T(key)))
		}
	}
	if r.Inverted {
		parts = append(parts, say.T("reading.overbass", chord.Spell(r.Bass, chord.Sharps).Format(no)))
	}
	return joinWords(say, parts)
}

// intervalKeys names the notes a chord can be missing by the degree a player
// counts: the interval in semitones is how the code thinks, and "the fifth" is
// how a person does. Both spellings of a degree land on the same word, because
// a flattened third is still the third.
var intervalKeys = map[int]string{
	2: "degree.ninth", 3: "degree.third", 4: "degree.third", 5: "degree.fourth",
	6: "degree.fifth", 7: "degree.fifth", 8: "degree.fifth", 9: "degree.sixth",
	10: "degree.seventh", 11: "degree.seventh", 14: "degree.ninth",
}

// joinWords strings a list together the way the language does. The last join is
// a word and not a comma, and which word it is belongs to the language.
func joinWords(say *i18n.Printer, parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1 : len(parts)-1] {
		out += ", " + p
	}
	return say.T("list.and", out, parts[len(parts)-1])
}

// variantsFor works out every way to play the chord that was asked for.
func variantsFor(say *i18n.Printer, o LabOptions) ([]LabShape, int, []InversionView, string, string) {
	if o.Symbol == "" {
		return nil, 0, nil, "", ""
	}
	c, err := chord.ParseIn(o.Notation, o.Symbol)
	if err != nil {
		if c, err = chord.Parse(o.Symbol); err != nil {
			return nil, 0, nil, "", say.T("lab.unreadable", o.Symbol)
		}
	}
	name := c.Format(o.Notation)
	g, ok := voicing.Generate(c, voicing.GenerateOptions{})
	if !ok {
		return nil, 0, nil, name, say.T("lab.unknownchord", name)
	}
	if len(g.Root.Shapes) == 0 && len(g.Inversions) == 0 {
		return nil, 0, nil, name, say.T("lab.unreachable", name)
	}

	var invs []InversionView
	for _, inv := range g.Inversions {
		if len(inv.Shapes) == 0 {
			continue
		}
		sym := inv.Chord.Format(chord.English)
		invs = append(invs, InversionView{
			Name:  inv.Chord.Format(o.Notation),
			Value: sym,
			URL:   labURL(sym, "", 0, o.All),
			Total: len(inv.Shapes),
		})
	}
	if len(g.Root.Shapes) == 0 {
		// Every way of holding it puts something other than the root underneath,
		// so there is nothing to show here and the honest thing is to name the
		// chord that does describe those shapes.
		return nil, 0, invs, name, say.T("lab.alwaysinverted", name)
	}
	shapes := shapeList(say, g.Root.Shapes, o)
	return shapes, len(g.Root.Shapes), invs, name, ""
}

// howMany is the length of the list of shapes.
//
// Twelve is enough to reach past the first position and see that the chord has a
// life further up the neck, without the page becoming a catalogue. Asking for
// more raises it rather than removing the limit: two hundred diagrams of one
// chord is not an answer to any question, and the real total is printed either
// way.
const (
	howMany    = 12
	howManyAll = 48
)

// shapeList describes the shapes in the order they are already in — easiest
// first — and stops when the page has enough.
func shapeList(say *i18n.Printer, vs []voicing.Voicing, o LabOptions) []LabShape {
	keep := howMany
	if o.All {
		keep = howManyAll
	}
	if keep > len(vs) {
		keep = len(vs)
	}
	out := make([]LabShape, 0, keep)
	for i, v := range vs[:keep] {
		view := voicingView(say, v, i, keep)
		out = append(out, LabShape{
			Shape: view,
			Note:  view.Name,
			Grip:  v.Grip(),
			Start: startFor(v),
			URL:   labURL(o.Symbol, v.Grip(), startFor(v), o.All),
		})
	}
	return out
}

// startFor is where the board should open so that a shape clicked in the list
// lands on it whole.
func startFor(v voicing.Voicing) int {
	w := windowFor(v)
	if w.Start > maxStart {
		return maxStart
	}
	return w.Start
}

// voicingView describes one shape for a template. It is the same description the
// song sheet uses, so a shape looks and reads the same wherever it appears.
func voicingView(say *i18n.Printer, v voicing.Voicing, i, total int) VoicingView {
	w := windowFor(v)
	vv := VoicingView{
		Name:       shapeName(say, v),
		StartFret:  w.Start,
		FretsShown: w.Frets,
		Index:      i + 1,
		Total:      total,
		ID:         v.ID(),
		Grip:       v.Grip(),
	}
	for _, f := range v.Frets {
		vv.Frets = append(vv.Frets, int(f))
	}
	for _, f := range v.Fingers {
		vv.Fingers = append(vv.Fingers, int(f))
	}
	for _, b := range v.Barres {
		vv.Barres = append(vv.Barres, BarreView{Fret: int(b.Fret), From: b.From, To: b.To})
	}
	vv.Fingering = describeShape(say, vv.Name, v)
	return vv
}

// chooserFor builds the two discoverable ways in to a chord, for a reader who
// would not think to type one.
// parseSymbol reads what was typed, in the reader's notation or in English,
// reporting whether it was a chord at all.
func parseSymbol(o LabOptions) (chord.Chord, bool) {
	if c, err := chord.ParseIn(o.Notation, o.Symbol); err == nil {
		return c, true
	}
	c, err := chord.Parse(o.Symbol)
	return c, err == nil
}

func chooserFor(say *i18n.Printer, o LabOptions) ([]Choice, []Choice) {
	c, _ := parseSymbol(o)
	suffix := c.Suffix

	roots := make([]Choice, 0, 12)
	for pc := range 12 {
		n := chord.Spell(pc, chord.SpellingFor(pc, false))
		sym := n.Format(chord.English) + suffix
		roots = append(roots, Choice{
			Label: n.Format(o.Notation),
			Value: sym,
			On:    c.Root.Valid() && c.Root.PitchClass() == pc,
		})
	}

	root := "C"
	if c.Root.Valid() {
		root = c.Root.Format(chord.English)
	}
	qualities := make([]Choice, 0, len(qualityChoices))
	for _, q := range qualityChoices {
		qualities = append(qualities, Choice{
			Label: say.T(q.key),
			Value: root + q.suffix,
			On:    suffix == q.suffix,
		})
	}
	return roots, qualities
}

// qualityChoices are the families offered by name, glossed so that somebody who
// does not read chord symbols can still find what they want.
//
// The suffix is the chord symbol and never translated: maj7 is maj7 in every
// language a guitarist reads. Only the gloss beside it is words.
var qualityChoices = []struct{ suffix, key string }{
	{"", "quality.major"},
	{"m", "quality.minor"},
	{"7", "quality.dom7"},
	{"maj7", "quality.maj7"},
	{"m7", "quality.min7"},
	{"mMaj7", "quality.minmaj7"},
	{"6", "quality.six"},
	{"m6", "quality.min6"},
	{"sus2", "quality.sus2"},
	{"sus4", "quality.sus4"},
	{"7sus4", "quality.dom7sus4"},
	{"add9", "quality.add9"},
	{"9", "quality.dom9"},
	{"maj9", "quality.maj9"},
	{"m9", "quality.min9"},
	{"dim", "quality.dim"},
	{"dim7", "quality.dim7"},
	{"m7b5", "quality.halfdim"},
	{"aug", "quality.aug"},
	{"5", "quality.power"},
}

func labURL(symbol, grip string, start int, all bool) string {
	q := url.Values{}
	if symbol != "" {
		q.Set("c", symbol)
	}
	if grip != "" {
		q.Set("grip", grip)
	}
	if start > 1 {
		q.Set("start", strconv.Itoa(start))
	}
	if all {
		q.Set("all", "1")
	}
	if len(q) == 0 {
		return "/chords"
	}
	return "/chords?" + q.Encode()
}

func labURLAt(o LabOptions, grip string, start int) string {
	return labURL(o.Symbol, grip, start, o.All)
}

// midiList writes note numbers the way an attribute carries them.
func midiList(midi []int) string {
	out := make([]string, len(midi))
	for i, m := range midi {
		out[i] = strconv.Itoa(m)
	}
	return strings.Join(out, ",")
}
