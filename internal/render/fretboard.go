package render

import (
	"cmp"
	"net/url"
	"slices"
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
type FretCell struct {
	Fret int
	On   bool
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
}

// FretboardView is the editable neck.
//
// Five frets, because that is what a diagram draws and the board has to match
// the pictures beside it. It is a form control and not a drawing: the choices
// are radio buttons, so the neck can be played with by keyboard, read aloud, and
// used with no JavaScript at all.
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

// PositionView is the shapes drawn on one window of the neck.
//
// Grouped by the window a diagram draws rather than by the lowest fretted note,
// so that every picture under one heading shows the same stretch of neck and the
// eye can compare them. Each shape keeps its own caption: the heading says where
// the hand goes, the caption says how it sits there.
type PositionView struct {
	Name   string // "Primera posición", "Traste 5"
	Total  int
	Shapes []LabShape
	start  int
}

// LabShape is one diagram in the list of ways to hold a chord.
//
// Note carries the shape's own caption only when it says something the heading
// above it does not: three pictures under "Primera posición" each labelled
// "Posición abierta" is noise, but "Cejilla en el traste 5" is the thing a
// player most needs to know before trying it.
type LabShape struct {
	Shape VoicingView
	Note  string
	URL   string
	On    bool // this is the shape currently on the neck
}

// GroupView is every shape for one written chord.
type GroupView struct {
	Name      string
	Total     int
	Positions []PositionView
}

// InversionView points at the same chord over a different bass, which is a
// different chord and gets its own page rather than being piled onto this one.
type InversionView struct {
	Name  string
	URL   string
	Total int
}

// LabView is the whole chord page.
type LabView struct {
	Symbol     string // what was asked for, as written
	Chord      string // what it was understood as, formatted
	Problem    string // an honest note when the input made no sense
	Board      FretboardView
	Readings   []ReadingView
	Sounds     []string // the notes drawn, when none of them make a chord
	Group      *GroupView
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
func Lab(o LabOptions) LabView {
	v := LabView{Symbol: o.Symbol, All: o.All}

	start := o.Start
	if start < 1 {
		start = 1
	}
	if start > maxStart {
		start = maxStart
	}

	frets, ok := voicing.ParseGrip(o.Grip)
	if !ok || len(frets) != voicing.Standard().Strings() {
		frets = make([]voicing.Fret, voicing.Standard().Strings())
		for i := range frets {
			frets[i] = voicing.Muted
		}
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

	v.Roots, v.Qualities = chooserFor(o)
	v.Group, v.Inversions, v.Chord, v.Problem = variantsFor(o)
	if v.Group != nil && !o.All {
		v.AllURL = labURL(o.Symbol, drawn.Grip(), start, true)
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
		}
		if i < len(stringNames) && len(v.Frets) == len(stringNames) {
			row.Name = stringNames[i]
		}
		for n := start; n < start+labFrets; n++ {
			row.Cells = append(row.Cells, FretCell{Fret: n, On: int(f) == n})
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
func variantsFor(o LabOptions) (*GroupView, []InversionView, string, string) {
	if o.Symbol == "" {
		return nil, nil, "", ""
	}
	c, err := chord.ParseIn(o.Notation, o.Symbol)
	if err != nil {
		if c, err = chord.Parse(o.Symbol); err != nil {
			return nil, nil, "", "No se entiende «" + o.Symbol + "» como un acorde."
		}
	}
	name := c.Format(o.Notation)
	g, ok := voicing.Generate(c, voicing.GenerateOptions{})
	if !ok {
		return nil, nil, name, "No se sabe formar " + name + " en la guitarra."
	}
	if len(g.Root.Shapes) == 0 && len(g.Inversions) == 0 {
		return nil, nil, name, "No hay ninguna forma de " + name + " que una mano alcance."
	}

	var invs []InversionView
	for _, inv := range g.Inversions {
		if len(inv.Shapes) == 0 {
			continue
		}
		invs = append(invs, InversionView{
			Name:  inv.Chord.Format(o.Notation),
			URL:   labURL(inv.Chord.Format(chord.English), "", 1, o.All),
			Total: len(inv.Shapes),
		})
	}
	if len(g.Root.Shapes) == 0 {
		// Every way of holding it puts something other than the root underneath,
		// so there is nothing to show here and the honest thing is to name the
		// chord that does describe those shapes.
		return nil, invs, name, "Toda forma de " + name + " deja otra nota en el bajo."
	}
	gv := groupView(name, g.Root.Shapes, o)
	return &gv, invs, name, ""
}

// perPosition is how many shapes one window of the neck is worth showing.
//
// Three is enough to see that a place has choices without turning the page into
// a wall of diagrams; asking for more raises it rather than removing the limit,
// because forty pictures of the same fret is not an answer to any question.
const (
	perPosition    = 3
	perPositionAll = 10
)

func groupView(name string, vs []voicing.Voicing, o LabOptions) GroupView {
	keep := perPosition
	if o.All {
		keep = perPositionAll
	}
	gv := GroupView{Name: name, Total: len(vs)}
	at := make(map[int]int)
	for _, v := range vs {
		w := windowFor(v)
		i, ok := at[w.Start]
		if !ok {
			gv.Positions = append(gv.Positions, PositionView{Name: windowName(w.Start), start: w.Start})
			i = len(gv.Positions) - 1
			at[w.Start] = i
		}
		p := &gv.Positions[i]
		p.Total++
		if len(p.Shapes) < keep {
			view := voicingView(v, len(p.Shapes), 0)
			note := ""
			if captionAdds(v) {
				note = view.Name
			}
			p.Shapes = append(p.Shapes, LabShape{
				Shape: view,
				Note:  note,
				URL:   labURL(o.Symbol, v.Grip(), startFor(v), o.All),
				On:    v.Grip() == o.Grip,
			})
		}
	}
	for i := range gv.Positions {
		for j := range gv.Positions[i].Shapes {
			gv.Positions[i].Shapes[j].Shape.Total = len(gv.Positions[i].Shapes)
		}
	}
	slices.SortFunc(gv.Positions, func(a, b PositionView) int { return cmp.Compare(a.start, b.start) })
	return gv
}

// captionAdds reports whether a shape's own caption says something its heading
// does not.
//
// The heading already gives the place, so "Posición abierta" under "Primera
// posición" and "Traste 5" under "Traste 5" are the same fact twice. A barre and
// an all-open shape are the two that are worth naming: they are what a player
// most wants to know before trying the grip.
func captionAdds(v voicing.Voicing) bool {
	switch windowFor(v).how {
	case barred, allOpen:
		return true
	default:
		return false
	}
}

// windowName says where on the neck a window sits. The nut end is named rather
// than numbered because that is what a player calls it.
func windowName(start int) string {
	if start <= 1 {
		return "Primera posición"
	}
	return "Traste " + strconv.Itoa(start)
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
