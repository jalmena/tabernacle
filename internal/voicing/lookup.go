package voicing

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
)

//go:embed data/chords.json
var dataFS embed.FS

// Playability limits. A shape outside these is not a chord voicing, it is a
// stretch nobody would attempt on a song sheet.
const (
	maxSpan     = 4
	maxPosition = 12
)

// wire mirrors the JSON dataset. It is decoded strictly, so a typo in the data
// file is a startup failure rather than a chord that silently disappears.
type wire struct {
	Version int    `json:"version"`
	Note    string `json:"note"`
	Curated []struct {
		Chord  string      `json:"chord"`
		Shapes []wireShape `json:"shapes"`
	} `json:"curated"`
	Movable []struct {
		Quality string      `json:"quality"`
		Shapes  []wireShape `json:"shapes"`
	} `json:"movable"`
}

type wireShape struct {
	ID      string `json:"id"`
	Root    int    `json:"root"`
	Frets   string `json:"frets"`
	Fingers string `json:"fingers"`
	Barre   *struct {
		Fret   int `json:"fret"`
		From   int `json:"from"`
		To     int `json:"to"`
		Finger int `json:"finger"`
	} `json:"barre"`
	MinFret int `json:"minFret"`
	MaxFret int `json:"maxFret"`
}

// qualityNames maps the dataset's quality keys onto families.
//
// They are spelled out rather than reusing each family's canonical symbol,
// because major's symbol is the empty string and an empty JSON key is a poor
// thing to ask a reviewer to notice.
var qualityNames = map[string]harmony.Quality{
	"major":    harmony.Major,
	"minor":    harmony.Minor,
	"dom7":     harmony.Dominant7,
	"maj7":     harmony.Major7,
	"min7":     harmony.Minor7,
	"minmaj7":  harmony.MinorMajor7,
	"dim":      harmony.Diminished,
	"dim7":     harmony.Diminished7,
	"min7b5":   harmony.HalfDiminished,
	"aug":      harmony.Augmented,
	"sus2":     harmony.Sus2,
	"sus4":     harmony.Sus4,
	"dom7sus4": harmony.Dominant7Sus4,
	"maj6":     harmony.Major6,
	"min6":     harmony.Minor6,
	"add9":     harmony.Add9,
	"dom9":     harmony.Dominant9,
	"maj9":     harmony.Major9,
	"min9":     harmony.Minor9,
	"power":    harmony.Power,
}

// key identifies a chord's physical identity: which pitches it needs, not how
// they are spelled.
//
// Keying on pitch class is what makes C#m7, Dbm7 and Do#m7 one entry rather than
// three. Db and C# are the same fingering, and the spelling is a presentation
// choice that must never decide which shape a click opens.
type key struct {
	root    int // pitch class 0..11
	quality harmony.Quality
	bass    int // pitch class, or -1 for no slash bass
}

// movableShape is a template ready to be slid up the neck.
type movableShape struct {
	id      string
	rootIdx int
	frets   []Fret // relative to the shape's own lowest line
	fingers []Finger
	barre   *Barre // fret is relative too
	minFret int
	maxFret int
}

type dataset struct {
	curated map[key][]Voicing
	movable map[harmony.Quality][]movableShape
}

// load reads and compiles the dataset once.
var load = sync.OnceValues(func() (*dataset, error) {
	raw, err := dataFS.ReadFile("data/chords.json")
	if err != nil {
		return nil, fmt.Errorf("voicing: read dataset: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	var w wire
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("voicing: decode dataset: %w", err)
	}
	if w.Version != 1 {
		return nil, fmt.Errorf("voicing: dataset version %d is not supported", w.Version)
	}

	ds := &dataset{
		curated: make(map[key][]Voicing),
		movable: make(map[harmony.Quality][]movableShape),
	}
	std := Standard()

	for _, entry := range w.Curated {
		c, err := chord.Parse(entry.Chord)
		if err != nil {
			return nil, fmt.Errorf("voicing: dataset chord %q: %w", entry.Chord, err)
		}
		q, red, ok := harmony.Of(c)
		if !ok {
			return nil, fmt.Errorf("voicing: dataset chord %q has an unclassifiable quality", entry.Chord)
		}
		if !red.Exact() {
			return nil, fmt.Errorf("voicing: dataset chord %q does not classify exactly", entry.Chord)
		}
		k := key{root: c.Root.PitchClass(), quality: q, bass: -1}
		if c.HasBass {
			k.bass = c.Bass.PitchClass()
		}
		for _, sh := range entry.Shapes {
			v, err := compileCurated(sh, std)
			if err != nil {
				return nil, fmt.Errorf("voicing: dataset chord %q shape %q: %w", entry.Chord, sh.ID, err)
			}
			ds.curated[k] = append(ds.curated[k], v)
		}
	}

	for _, entry := range w.Movable {
		q, ok := qualityNames[entry.Quality]
		if !ok {
			return nil, fmt.Errorf("voicing: dataset names an unknown quality %q", entry.Quality)
		}
		for _, sh := range entry.Shapes {
			m, err := compileMovable(sh)
			if err != nil {
				return nil, fmt.Errorf("voicing: movable %s/%s: %w", entry.Quality, sh.ID, err)
			}
			ds.movable[q] = append(ds.movable[q], m)
		}
	}
	return ds, nil
})

func compileCurated(sh wireShape, tuning Tuning) (Voicing, error) {
	frets, err := parseFrets(sh.Frets)
	if err != nil {
		return Voicing{}, err
	}
	if len(frets) != tuning.Strings() {
		return Voicing{}, fmt.Errorf("%d frets for %d strings", len(frets), tuning.Strings())
	}
	fingers, err := parseFingers(sh.Fingers, len(frets))
	if err != nil {
		return Voicing{}, err
	}
	v := Voicing{
		Tuning:  tuning,
		Frets:   frets,
		Fingers: fingers,
		Shape:   sh.ID,
	}
	if sh.Barre != nil {
		v.Barres = []Barre{{
			Fret:   Fret(sh.Barre.Fret),
			From:   sh.Barre.From,
			To:     sh.Barre.To,
			Finger: Finger(sh.Barre.Finger),
		}}
	}
	v.Position = lowestFret(frets)
	v.Cost = cost(v)
	return v, nil
}

func compileMovable(sh wireShape) (movableShape, error) {
	frets, err := parseFrets(sh.Frets)
	if err != nil {
		return movableShape{}, err
	}
	fingers, err := parseFingers(sh.Fingers, len(frets))
	if err != nil {
		return movableShape{}, err
	}
	if sh.Root < 0 || sh.Root >= len(frets) {
		return movableShape{}, fmt.Errorf("root index %d is not a string", sh.Root)
	}
	if frets[sh.Root] < 0 {
		return movableShape{}, fmt.Errorf("the root string is muted")
	}
	m := movableShape{
		id:      sh.ID,
		rootIdx: sh.Root,
		frets:   frets,
		fingers: fingers,
		minFret: sh.MinFret,
		maxFret: sh.MaxFret,
	}
	if m.minFret < 1 {
		// Never at the nut: a template's relative zero means "the barre line",
		// and at the nut that silently becomes "open string", which changes both
		// the sound and the fingering. Open shapes are curated instead.
		m.minFret = 1
	}
	if m.maxFret <= 0 || m.maxFret > maxPosition {
		m.maxFret = maxPosition
	}
	if sh.Barre != nil {
		m.barre = &Barre{
			Fret:   Fret(sh.Barre.Fret),
			From:   sh.Barre.From,
			To:     sh.Barre.To,
			Finger: Finger(sh.Barre.Finger),
		}
	}
	return m, nil
}

// period returns how far a chord family has to be transposed before it sounds
// the same set of notes again: twelve for almost everything, but three for a
// diminished seventh and four for an augmented triad.
//
// This matters because a symmetric shape voices the same chord at several places
// on the neck. Without it, Cdim7 would be offered only at the tenth fret, when
// the identical grip works at the first, fourth and seventh as well.
func period(q harmony.Quality) int {
	tones := q.Intervals()
	if len(tones) == 0 {
		return 12
	}
	set := map[int]bool{}
	for _, t := range tones {
		set[((t%12)+12)%12] = true
	}
	for k := 1; k < 12; k++ {
		same := true
		for p := range set {
			if !set[(p+k)%12] {
				same = false
				break
			}
		}
		if same {
			return k
		}
	}
	return 12
}

// slide places a movable template so that its root sounds the wanted pitch
// class, returning one voicing per position that fits.
func (m movableShape) slide(rootPC int, tuning Tuning) []Voicing {
	return m.slideQuality(rootPC, 12, tuning)
}

// slideQuality is slide with the family's repetition period, so that a symmetric
// chord is offered everywhere its grip actually works.
func (m movableShape) slideQuality(rootPC, step int, tuning Tuning) []Voicing {
	if len(m.frets) != tuning.Strings() {
		return nil
	}
	if step < 1 || step > 12 {
		step = 12
	}
	base := tuning.Open[m.rootIdx] + int(m.frets[m.rootIdx])
	offset := ((rootPC-base)%12 + 12) % 12

	// Start at the lowest position congruent to the offset, so a symmetric shape
	// is found low on the neck rather than only where the arithmetic first lands.
	start := offset % step
	if start < m.minFret {
		start += step
	}

	var out []Voicing
	for pos := start; pos <= m.maxFret; pos += step {
		if pos < m.minFret {
			continue
		}
		frets := make([]Fret, len(m.frets))
		tooHigh := false
		for i, f := range m.frets {
			if f == Muted {
				frets[i] = Muted
				continue
			}
			abs := int(f) + pos
			if abs > 24 {
				tooHigh = true
				break
			}
			frets[i] = Fret(abs)
		}
		if tooHigh {
			continue
		}
		fingers := make([]Finger, len(m.fingers))
		copy(fingers, m.fingers)

		v := Voicing{
			Tuning:  tuning,
			Frets:   frets,
			Fingers: fingers,
			Shape:   m.id,
		}
		if m.barre != nil {
			v.Barres = []Barre{{
				Fret:   m.barre.Fret + Fret(pos),
				From:   m.barre.From,
				To:     m.barre.To,
				Finger: m.barre.Finger,
			}}
		}
		v.Position = lowestFret(frets)
		v.Cost = cost(v)
		out = append(out, v)
	}
	return out
}

func lowestFret(frets []Fret) int {
	lo := 0
	for _, f := range frets {
		if f <= 0 {
			continue
		}
		if lo == 0 || int(f) < lo {
			lo = int(f)
		}
	}
	return lo
}

// attempt is one step of the degradation chain.
type attempt struct {
	key   key
	kind  ApproxKind
	given string
}

// For returns the shapes that play a chord.
//
// The chain of attempts is built up front so that falling back is a loop rather
// than nested special cases. A slash bass is dropped before the quality is
// touched, because the bass is usually an inversion somebody else is covering
// whereas the quality is the chord's identity.
//
// Exact shapes are never mixed with approximate ones: "3 de 5" must not mean
// three real shapes and two invented.
func For(c chord.Chord, opts Options) Result {
	tuning := opts.Tuning
	if tuning.IsZero() {
		tuning = Standard()
	}
	if opts.Max <= 0 {
		opts.Max = DefaultMax
	}

	res := Result{Symbol: c.String()}

	ds, err := load()
	if err != nil {
		res.Warning = "No se ha podido cargar la base de acordes."
		return res
	}

	q, red, ok := harmony.Of(c)
	if !ok {
		res.Warning = "No hay una posición conocida para este acorde."
		return res
	}
	res.Quality = q

	rootPC := c.Root.PitchClass()
	bassPC := -1
	if c.HasBass {
		bassPC = c.Bass.PitchClass()
	}

	var attempts []attempt
	add := func(k key, kind ApproxKind, given string) {
		attempts = append(attempts, attempt{key: k, kind: kind, given: given})
	}

	// What was asked for, allowing for decoration this package does not model.
	firstKind := Exact
	firstGiven := c.String()
	if !red.Exact() {
		firstKind = QualitySimplified
		simple := c
		simple.Suffix = q.Symbol()
		firstGiven = simple.String()
	}
	add(key{rootPC, q, bassPC}, firstKind, firstGiven)

	if bassPC >= 0 {
		noBass := c
		noBass.HasBass = false
		noBass.Suffix = q.Symbol()
		add(key{rootPC, q, -1}, BassOmitted, noBass.String())
	}

	// Then walk the simplification chain, with and without the bass.
	cur := q
	for {
		parent, has := cur.Parent()
		if !has {
			break
		}
		simple := c
		simple.Suffix = parent.Symbol()
		if bassPC >= 0 {
			add(key{rootPC, parent, bassPC}, QualitySimplified, simple.String())
			noBass := simple
			noBass.HasBass = false
			add(key{rootPC, parent, -1}, QualitySimplified, noBass.String())
		} else {
			add(key{rootPC, parent, -1}, QualitySimplified, simple.String())
		}
		cur = parent
	}

	for _, at := range attempts {
		found := gather(ds, at, tuning, opts)
		if len(found) == 0 {
			continue
		}
		found = dedupe(found)
		sort.SliceStable(found, func(i, j int) bool {
			if found[i].Cost != found[j].Cost {
				return found[i].Cost < found[j].Cost
			}
			return found[i].Position < found[j].Position
		})
		found = thin(found, opts.Max)
		res.Voicings = found
		res.Warning = warningFor(found, tuning)
		return res
	}

	res.Warning = warningForNothing(tuning)
	return res
}

// gather collects every candidate for one attempt.
func gather(ds *dataset, at attempt, tuning Tuning, opts Options) []Voicing {
	var out []Voicing
	std := Standard()

	appendIf := func(v Voicing, from Tuning) {
		if !compatible(from, tuning, v.Frets) {
			return
		}
		if v.Span() > maxSpan || v.Position > maxPosition {
			return
		}
		v.Tuning = tuning
		v.Approx = Approximation{Kind: at.kind, Asked: at.key.quality.Symbol(), Given: at.given}
		if at.kind == Exact && !from.Equal(tuning) {
			v.Approx.Kind = ForeignTuning
		}
		out = append(out, v)
	}

	for _, v := range ds.curated[at.key] {
		appendIf(v, v.Tuning)
	}
	// Movable templates only voice a chord with no slash bass: sliding a shape
	// does not give it a specified lowest note.
	if at.key.bass < 0 {
		for _, m := range ds.movable[at.key.quality] {
			// Templates are written for standard tuning; compatibility is checked
			// per shape, which is what makes Drop D nearly free.
			for _, v := range m.slideQuality(at.key.root, period(at.key.quality), std) {
				appendIf(v, std)
			}
		}
	}
	return out
}

// dedupe removes shapes with identical fret vectors. Curated and movable entries
// overlap constantly at higher positions, and curated wins because it carries
// the idiomatic fingering.
func dedupe(vs []Voicing) []Voicing {
	seen := make(map[string]bool, len(vs))
	out := vs[:0]
	for _, v := range vs {
		k := fretKey(v.Frets)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

func fretKey(frets []Fret) string {
	var b strings.Builder
	for _, f := range frets {
		fmt.Fprintf(&b, "%d,", f)
	}
	return b.String()
}

// thin keeps the list short and varied: at most one shape per family and
// position, and positions at least two frets apart, so the reader is offered
// genuinely different options rather than the same barre inched along the neck.
func thin(vs []Voicing, max int) []Voicing {
	var out []Voicing
	usedShape := map[string]bool{}
	for _, v := range vs {
		if len(out) >= max {
			break
		}
		if usedShape[v.Shape] && v.Position != 0 {
			tooClose := false
			for _, kept := range out {
				if kept.Shape == v.Shape && abs(kept.Position-v.Position) < 3 {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
		}
		usedShape[v.Shape] = true
		out = append(out, v)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// cost scores playability. The weights matter less than two properties, both
// pinned by tests: an open shape always beats a barre, and the order is
// deterministic, because the interface says "2 de 4" and a deep link names a
// shape.
func cost(v Voicing) int {
	c := 0
	fretted, muted, thumb := 0, 0, 0
	for i, f := range v.Frets {
		switch {
		case f == Muted:
			muted++
		case f > 0:
			fretted++
		}
		if i < len(v.Fingers) && v.Fingers[i] == Thumb {
			thumb++
		}
	}
	c += 4 * fretted
	if s := v.Span(); s > 1 {
		// A stretch is what actually stops a hand, more than the number of
		// fingers involved.
		c += 8 * (s - 1)
	}
	if len(v.Barres) > 0 {
		c += 6
		width := v.Barres[0].To - v.Barres[0].From + 1
		c += width
		// The neck is thickest and the tension highest at the first two frets,
		// which is why a player manages F at the fifth fret long before the
		// first.
		if v.Position <= 2 && width >= 5 {
			c += 4
		}
	}
	// Position matters a lot: a shape high up is harder to reach and harder to
	// read at arm's length, and it is not what a chord reference prints first.
	c += 3 * v.Position
	c += 6 * thumb
	// A muted string between two sounded ones has to be damped deliberately.
	c += 8 * mutedInside(v.Frets)
	// A fuller voicing is the better primary choice for a song sheet: it is the
	// bigger sound, and it is the shape people expect to be shown. Without this
	// a thin four-string grip wins simply by fretting fewer notes.
	c += 6 * muted
	return c
}

// mutedInside counts muted strings with sounded strings on both sides.
func mutedInside(frets []Fret) int {
	first, last := -1, -1
	for i, f := range frets {
		if f != Muted {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return 0
	}
	n := 0
	for i := first; i <= last; i++ {
		if frets[i] == Muted {
			n++
		}
	}
	return n
}

// warningFor explains an approximate result, in Spanish and ready to display.
func warningFor(vs []Voicing, tuning Tuning) string {
	if len(vs) == 0 {
		return warningForNothing(tuning)
	}
	a := vs[0].Approx
	switch a.Kind {
	case Exact:
		return ""
	case BassOmitted:
		return "Se muestra " + a.Given + ", sin el bajo indicado."
	case ForeignTuning:
		// Not a compromise on the chord itself, just where the shape came from.
		// In a song with a non-standard tuning this applies to most chords, so
		// it is said once for the whole sheet rather than on every card.
		return ""
	default:
		return "No hay posición exacta: se muestra " + a.Given + "."
	}
}

func warningForNothing(tuning Tuning) string {
	if !tuning.Equal(Standard()) {
		return "No hay posiciones para la afinación " + tuning.Name + "."
	}
	return "No hay una posición conocida para este acorde."
}

// Dataset exposes the compiled dataset for the validator test. It is not part of
// the package's working interface.
func loadForTest() (*dataset, error) { return load() }
