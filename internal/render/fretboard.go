package render

import (
	"net/url"
	"strconv"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// LabOptions is what the chord page needs in order to answer.
type LabOptions struct {
	Notation chord.Notation
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
}

// ReadingView is one name for the notes drawn on the board.
type ReadingView struct {
	Name string
	URL  string
	Note string // "sin la quinta", "con Mi en el bajo"
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
	v := LabView{Symbol: o.Symbol, All: o.All}
	v.Roots, v.Qualities = chooserFor(o)
	v.Shapes, v.Total, v.Inversions, v.Chord, v.Problem = variantsFor(o)

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
	v.Board = boardFor(drawn, start, o)

	for _, r := range voicing.Identify(frets, voicing.Standard()) {
		v.Readings = append(v.Readings, ReadingView{
			Name: r.Chord.Format(o.Notation),
			URL:  labURL(r.Chord.Format(chord.English), drawn.Grip(), start, o.All),
			Note: readingNote(r, o.Notation),
		})
	}
	if len(v.Readings) == 0 {
		for _, pc := range drawn.PitchClasses() {
			v.Sounds = append(v.Sounds, chord.Spell(pc, chord.Sharps).Format(o.Notation))
		}
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
func boardFor(v voicing.Voicing, start int, o LabOptions) FretboardView {
	b := FretboardView{Start: start, Grip: v.Grip()}
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
		if i < len(stringNames) && len(v.Frets) == len(stringNames) {
			row.Name = stringNames[i]
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

// readingNote says what is unusual about a reading, so that "Do7 sin la quinta"
// is not silently presented as an ordinary Do7.
func readingNote(r voicing.Reading, no chord.Notation) string {
	var parts []string
	for _, iv := range r.Omitted {
		if name, ok := intervalNames[iv]; ok {
			parts = append(parts, "sin "+name)
		}
	}
	if r.Inverted {
		parts = append(parts, "con "+chord.Spell(r.Bass, chord.Sharps).Format(no)+" en el bajo")
	}
	return joinWords(parts)
}

// intervalNames names the notes a chord can be missing, in Spanish and in the
// degree a player counts: the interval in semitones is how the code thinks, and
// "la quinta" is how a person does.
var intervalNames = map[int]string{
	3: "la tercera", 4: "la tercera", 7: "la quinta",
	10: "la séptima", 11: "la séptima", 9: "la sexta",
	2: "la novena", 14: "la novena", 5: "la cuarta",
	6: "la quinta", 8: "la quinta",
}

func joinWords(parts []string) string {
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
	return out + " y " + parts[len(parts)-1]
}

// variantsFor works out every way to play the chord that was asked for.
func variantsFor(o LabOptions) ([]LabShape, int, []InversionView, string, string) {
	if o.Symbol == "" {
		return nil, 0, nil, "", ""
	}
	c, err := chord.ParseIn(o.Notation, o.Symbol)
	if err != nil {
		if c, err = chord.Parse(o.Symbol); err != nil {
			return nil, 0, nil, "", "No se entiende «" + o.Symbol + "» como un acorde."
		}
	}
	name := c.Format(o.Notation)
	g, ok := voicing.Generate(c, voicing.GenerateOptions{})
	if !ok {
		return nil, 0, nil, name, "No se sabe formar " + name + " en la guitarra."
	}
	if len(g.Root.Shapes) == 0 && len(g.Inversions) == 0 {
		return nil, 0, nil, name, "No hay ninguna forma de " + name + " que una mano alcance."
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
		return nil, 0, invs, name, "Toda forma de " + name + " deja otra nota en el bajo."
	}
	shapes := shapeList(g.Root.Shapes, o)
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
func shapeList(vs []voicing.Voicing, o LabOptions) []LabShape {
	keep := howMany
	if o.All {
		keep = howManyAll
	}
	if keep > len(vs) {
		keep = len(vs)
	}
	out := make([]LabShape, 0, keep)
	for i, v := range vs[:keep] {
		view := voicingView(v, i, keep)
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
func voicingView(v voicing.Voicing, i, total int) VoicingView {
	w := windowFor(v)
	vv := VoicingView{
		Name:       shapeName(v),
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
	vv.Fingering = describeShape(vv.Name, v)
	return vv
}

// chooserFor builds the two discoverable ways in to a chord, for a reader who
// would not think to type one.
func chooserFor(o LabOptions) ([]Choice, []Choice) {
	c, err := chord.ParseIn(o.Notation, o.Symbol)
	if err != nil {
		c, _ = chord.Parse(o.Symbol)
	}
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
			Label: q.label,
			Value: root + q.suffix,
			On:    suffix == q.suffix,
		})
	}
	return roots, qualities
}

// qualityChoices are the families offered by name, glossed so that somebody who
// does not read chord symbols can still find what they want.
var qualityChoices = []struct{ suffix, label string }{
	{"", "mayor"},
	{"m", "menor"},
	{"7", "séptima de dominante"},
	{"maj7", "séptima mayor"},
	{"m7", "séptima menor"},
	{"mMaj7", "menor con séptima mayor"},
	{"6", "sexta"},
	{"m6", "sexta menor"},
	{"sus2", "cuarta suspendida (sus2)"},
	{"sus4", "cuarta suspendida (sus4)"},
	{"7sus4", "séptima suspendida"},
	{"add9", "con novena añadida"},
	{"9", "novena de dominante"},
	{"maj9", "novena mayor"},
	{"m9", "novena menor"},
	{"dim", "disminuido"},
	{"dim7", "séptima disminuida"},
	{"m7b5", "semidisminuido"},
	{"aug", "aumentado"},
	{"5", "quinta (acorde de poder)"},
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
