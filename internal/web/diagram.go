package web

import (
	"html/template"
	"strings"

	"github.com/jalmena/tabernacle/internal/diagram"
	"github.com/jalmena/tabernacle/internal/render"
)

// chordDiagram renders a chord shape as inline SVG.
//
// This is the single place in the application that hands a template
// pre-escaped markup, and the exception is deliberate and narrow. The rule it
// breaks exists to keep user-supplied text out of trusted markup; this output is
// generated from integers by a package that accepts no markup at all and escapes
// the one string it does take, which its own tests assert. Drawing the geometry
// in a template instead would duplicate it somewhere it cannot be golden-tested.
//
// TestOnlyDiagramProducesMarkup keeps the exception to this function.
func chordDiagram(v render.VoicingView, label string) template.HTML {
	var b strings.Builder
	_ = diagram.Inline(&b, diagram.Input{
		Strings:   len(v.Frets),
		Frets:     v.Frets,
		Fingers:   v.Fingers,
		Barres:    barresFor(v),
		StartFret: v.StartFret,
		Label:     label,
		Title:     v.Fingering,
	})
	return template.HTML(b.String())
}

func barresFor(v render.VoicingView) []diagram.Barre {
	if len(v.Barres) == 0 {
		return nil
	}
	out := make([]diagram.Barre, 0, len(v.Barres))
	for _, b := range v.Barres {
		out = append(out, diagram.Barre{Fret: b.Fret, From: b.From, To: b.To})
	}
	return out
}
