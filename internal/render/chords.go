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
	Fingering string // the shape described in words, for a screen reader
	Index     int    // 1-based, for "2 de 4"
	Total     int
	ID        string
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
		vv := VoicingView{
			Name:      shapeName(v),
			StartFret: windowStart(v),
			Index:     i + 1,
			Total:     len(res.Voicings),
			ID:        v.ID(),
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

// windowStart is the fret the diagram's window should begin at.
//
// An open shape starts at the nut. A shape whose notes all fit within the first
// few frets is still drawn from the nut, because seeing the nut is how a reader
// knows where they are; only a shape further up starts higher.
func windowStart(v voicing.Voicing) int {
	if v.Position <= 1 {
		return 1
	}
	hi := 0
	for _, f := range v.Frets {
		if int(f) > hi {
			hi = int(f)
		}
	}
	if hi <= 5 {
		return 1
	}
	return v.Position
}

// shapeName says where on the neck a shape sits, which is what a player wants to
// know before looking at the dots.
func shapeName(v voicing.Voicing) string {
	if v.Position == 0 {
		return "Cuerdas al aire"
	}
	if v.Position <= 1 && !v.HasBarre() {
		return "Posición abierta"
	}
	if v.HasBarre() {
		return "Cejilla en el traste " + strconv.Itoa(int(v.Barres[0].Fret))
	}
	if v.Position == 1 {
		return "Posición abierta"
	}
	return "Traste " + strconv.Itoa(v.Position)
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
