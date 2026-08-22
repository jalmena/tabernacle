package render

import (
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// VoicingView is one playable shape, ready for a template.
//
// The fret and finger numbers are plain integers rather than the voicing
// package's own types, so that a template and the diagram renderer need not know
// anything about how chords are modelled.
type VoicingView struct {
	Name      string // "Posición abierta", "Cejilla en el traste 5"
	Frets     []int  // lowest string first; -1 muted, 0 open
	Fingers   []int
	Barres    []BarreView
	StartFret int
	// FretsShown is how many frets the diagram draws. It travels with StartFret
	// because the two are one decision; see Window.
	FretsShown int
	Fingering  string // the shape described in words, for a screen reader
	Index      int    // 1-based, for "2 de 4"
	Total      int
	ID         string
	// Grip is the shape as a chord chart writes it, "x-3-2-0-1-0". It is how a
	// shape travels in a URL, so that one clicked in a list can be put back on
	// the editable neck exactly as it was.
	Grip string
}

// BarreView is a barre, flattened for a template.
type BarreView struct {
	Fret     int
	From, To int
}

// chordVoicings looks up the shapes for one chord and describes them.
func chordVoicings(c chord.Chord, text string, opts voicing.Options) ([]VoicingView, string) {
	res := voicing.For(c, opts)
	out := make([]VoicingView, 0, len(res.Voicings))
	for i, v := range res.Voicings {
		w := windowFor(v)
		vv := VoicingView{
			Name:       shapeName(v),
			StartFret:  w.Start,
			FretsShown: w.Frets,
			Index:      i + 1,
			Total:      len(res.Voicings),
			ID:         v.ID(),
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
		vv.Fingering = describeShape(text, v)
		out = append(out, vv)
	}
	return out, res.Warning
}

// windowFrets is how many frets a diagram draws.
//
// Five. A shape spans at most four, which internal/voicing enforces, so five
// always holds the whole shape and leaves the grid room to breathe.
const windowFrets = 5

// firstPositionFrets is how far up the neck first position reaches: a hand at the
// nut covers four frets.
//
// This is a fact about hands, not about drawings, and it is deliberately not
// windowFrets. How many frets a picture shows is a matter of taste; what counts
// as open position is not.
const firstPositionFrets = 4

// placement is the kind of place a shape sits in. It decides both the caption and
// where the diagram starts, so the words and the picture cannot disagree.
type placement int

const (
	// allOpen: nothing is fretted at all.
	allOpen placement = iota
	// barred: a finger lies across several strings.
	barred
	// firstPosition: open strings, hand at the nut. What a chord book calls an
	// open chord.
	firstPosition
	// closed: no open strings, so the shape is named by where the hand is.
	closed
)

// Window is where a diagram of a shape begins and how many frets it shows.
//
// The caption comes from the same Window, so a caption can never name a fret the
// picture does not show.
type Window struct {
	Start int
	Frets int
	how   placement
	// names is the fret the caption mentions: the barre's on a barred shape, the
	// hand's on a closed one. Unused by the two that name no fret.
	names int
}

// windowFor decides where a shape sits, and therefore both how it is drawn and
// what it is called.
//
// The window always begins at the lowest fretted note, because that is the note
// furthest from the reader's eye and the one a cut would lose. What the caption
// *names* can be a different fret: an A-shape barre is x 3 5 5 5 5, held with the
// index at the third fret and a barre at the fifth, and a player calls that a
// barre at the fifth while the picture has to start at the third.
//
// The order of the tests matters. A barre is tested before open strings because a
// barre chord is never an open chord: F sits at the first fret and no chord book
// calls it open. Needing an open string comes next, because that — and not owning
// a first-fret note — is what open position means. G is 3 2 0 0 0 3 and is as
// open as C.
func windowFor(v voicing.Voicing) Window {
	switch {
	case v.Reach() == 0:
		return Window{Start: 1, Frets: windowFrets, how: allOpen}
	case v.HasBarre():
		return Window{Start: v.Position, Frets: windowFrets, how: barred, names: lowestBarre(v)}
	case v.HasOpenString() && v.Reach() <= firstPositionFrets:
		return Window{Start: 1, Frets: windowFrets, how: firstPosition}
	default:
		// Named by where the index finger sits, which is what "position" means to
		// a player. A shape with no open strings is closed even at the first
		// fret.
		return Window{Start: v.Position, Frets: windowFrets, how: closed, names: v.Position}
	}
}

// lowestBarre is the fret of the barre nearest the nut, which is the one a player
// names the grip after.
func lowestBarre(v voicing.Voicing) int {
	lo := int(v.Barres[0].Fret)
	for _, b := range v.Barres[1:] {
		if int(b.Fret) < lo {
			lo = int(b.Fret)
		}
	}
	return lo
}

// shapeName says where on the neck a shape sits, which is what a player wants to
// know before looking at the dots.
func shapeName(v voicing.Voicing) string {
	w := windowFor(v)
	switch w.how {
	case allOpen:
		return "Cuerdas al aire"
	case barred:
		return "Cejilla en el traste " + strconv.Itoa(w.names)
	case firstPosition:
		return "Posición abierta"
	default:
		return "Traste " + strconv.Itoa(w.names)
	}
}

// stringNames are the guitar's strings as a player counts them: the sixth is the
// lowest. The dataset is indexed the other way round, lowest first, so this
// reverses on the way out rather than keeping two conventions in the code.
var stringNames = [6]string{"6ª", "5ª", "4ª", "3ª", "2ª", "1ª"}

var fingerNames = map[int]string{
	1: "índice", 2: "medio", 3: "anular", 4: "pequeño", 5: "pulgar",
}

// describeShape puts a shape into words.
//
// This is the accessible content of a diagram, and it is also what makes the
// panel useful on a phone where the picture is small. Producing it here rather
// than in JavaScript keeps the one description in tested Go.
func describeShape(name string, v voicing.Voicing) string {
	var parts []string
	for i, f := range v.Frets {
		label := "cuerda " + strconv.Itoa(i+1)
		if i < len(stringNames) && len(v.Frets) == 6 {
			label = stringNames[i] + " cuerda"
		}
		switch {
		case f == voicing.Muted:
			parts = append(parts, label+" al aire no suena")
		case f == voicing.Open:
			parts = append(parts, label+" al aire")
		default:
			s := label + " traste " + strconv.Itoa(int(f))
			if i < len(v.Fingers) {
				if fn, ok := fingerNames[int(v.Fingers[i])]; ok {
					s += " con el " + fn
				}
			}
			parts = append(parts, s)
		}
	}
	prefix := name
	if prefix != "" {
		prefix += ": "
	}
	return prefix + strings.Join(parts, ", ") + "."
}
