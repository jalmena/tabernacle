// Package diagram draws a guitar chord diagram as SVG.
//
// It imports nothing from the rest of Tabernacle, not even the chord types. That
// is the point: a diagram is a picture of fret numbers, and keeping it ignorant
// of how chords are represented means the two cannot become entangled. The only
// place notation enters is Input.Label, and it enters as a finished string.
//
// Nothing here emits a colour. Every stroke and fill is currentColor, so the
// same markup is correct in a light theme, a dark theme and on paper, and the
// golden test files do not churn when the palette changes.
package diagram

import (
	"fmt"
	"io"
	"strings"
)

// Fret values with a special meaning.
const (
	Muted = -1 // the string is not sounded
	Open  = 0  // the string is sounded unfretted
)

// Barre is one finger holding several strings down at one fret.
type Barre struct {
	Fret     int // absolute, counted from the nut
	From, To int // string indices, inclusive, lowest string first
	Finger   int
}

// Input is everything needed to draw one diagram.
//
// Frets and Fingers are indexed lowest string first, which is the order chord
// grids are written in. Frets are absolute; StartFret says which fret the top of
// the drawn window is, so a shape high on the neck is drawn in a five-fret
// window with its position labelled rather than as a very tall picture.
type Input struct {
	Strings   int
	Frets     []int
	Fingers   []int // 0 where there is no finger; 5 means the thumb
	Barres    []Barre
	StartFret int    // first fret shown; 1 for an open-position shape
	Window    int    // frets shown; 0 means the default of 5
	Label     string // chord name as it should read, already in the right notation
	Title     string // accessible description
}

// Geometry, in the SVG's own coordinate space. A viewBox means the caller sizes
// the diagram in CSS and never has to think about these numbers.
const (
	stringGap     = 12
	fretGap       = 14
	padX          = 11
	padTop        = 17 // room for the open and muted markers above the nut
	padBottom     = 15 // room for the finger numbers below
	dotRadius     = 4.2
	defaultWindow = 5
)

// Inline writes a diagram for embedding in an HTML document.
//
// No xmlns attribute: inside HTML the SVG namespace is implied, and including it
// is merely noise repeated once per diagram.
func Inline(w io.Writer, in Input) error { return write(w, in, false) }

// Standalone writes a diagram as a complete SVG document.
func Standalone(w io.Writer, in Input) error { return write(w, in, true) }

// String renders a diagram to a string, for a template that needs one value.
func String(in Input) string {
	var b strings.Builder
	_ = Inline(&b, in)
	return b.String()
}

func write(w io.Writer, in Input, standalone bool) error {
	in = normalize(in)

	gridW := float64(in.Strings-1) * stringGap
	totalW := gridW + 2*padX
	gridH := float64(in.Window) * fretGap
	totalH := padTop + gridH + padBottom

	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 `)
	num(&b, totalW)
	b.WriteByte(' ')
	num(&b, totalH)
	b.WriteString(`" class="chorddiagram" role="img"`)
	if standalone {
		b.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
	}
	b.WriteString(` fill="currentColor" stroke="currentColor">`)

	title := in.Title
	if title == "" {
		title = in.Label
	}
	if t := escaped(title); t != "" {
		b.WriteString(`<title>`)
		b.WriteString(t)
		b.WriteString(`</title>`)
	}

	x := func(i int) float64 { return padX + float64(i)*stringGap }
	// The y of the centre of the cell holding fret f.
	yFor := func(f int) float64 {
		return padTop + (float64(f-in.StartFret)+0.5)*fretGap
	}

	// The grid, as two paths rather than a dozen lines: the same picture in a
	// fifth of the bytes, and still a self-contained diagram.
	b.WriteString(`<path stroke-width="0.7" fill="none" d="`)
	for i := 0; i < in.Strings; i++ {
		b.WriteString("M")
		num(&b, x(i))
		b.WriteByte(' ')
		num(&b, padTop)
		b.WriteString("V")
		num(&b, padTop+gridH)
	}
	for j := 0; j <= in.Window; j++ {
		y := padTop + float64(j)*fretGap
		b.WriteString("M")
		num(&b, padX)
		b.WriteByte(' ')
		num(&b, y)
		b.WriteString("H")
		num(&b, padX+gridW)
	}
	b.WriteString(`"/>`)

	if in.StartFret <= 1 {
		// The nut: a thick line, which is what tells a reader at a glance that
		// this is an open-position shape.
		b.WriteString(`<path stroke-width="3" stroke-linecap="butt" d="M`)
		num(&b, padX-0.2)
		b.WriteByte(' ')
		num(&b, padTop)
		b.WriteString("H")
		num(&b, padX+gridW+0.2)
		b.WriteString(`"/>`)
	} else {
		// Otherwise say which fret the window starts at, or the diagram is a
		// shape with no position.
		b.WriteString(`<text x="`)
		num(&b, padX-4.5)
		b.WriteString(`" y="`)
		num(&b, yFor(in.StartFret)+2.4)
		b.WriteString(`" font-size="8" text-anchor="end" stroke="none">`)
		fmt.Fprintf(&b, "%d", in.StartFret)
		b.WriteString(`</text>`)
	}

	// Barres are drawn before the dots so the dots sit on top of them.
	for _, br := range in.Barres {
		if br.From < 0 || br.To >= in.Strings || br.From > br.To {
			continue
		}
		if br.Fret < in.StartFret || br.Fret >= in.StartFret+in.Window {
			continue
		}
		y := yFor(br.Fret)
		b.WriteString(`<rect x="`)
		num(&b, x(br.From)-dotRadius)
		b.WriteString(`" y="`)
		num(&b, y-dotRadius)
		b.WriteString(`" width="`)
		num(&b, x(br.To)-x(br.From)+2*dotRadius)
		b.WriteString(`" height="`)
		num(&b, 2*dotRadius)
		b.WriteString(`" rx="`)
		num(&b, dotRadius)
		b.WriteString(`" stroke="none"/>`)
	}

	for i, f := range in.Frets {
		switch {
		case f == Muted:
			// A cross above the nut.
			cx, cy := x(i), padTop-7.5
			const r = 3.0
			b.WriteString(`<path stroke-width="1.3" fill="none" d="M`)
			num(&b, cx-r)
			b.WriteByte(' ')
			num(&b, cy-r)
			b.WriteString("l")
			num(&b, 2*r)
			b.WriteByte(' ')
			num(&b, 2*r)
			b.WriteString("M")
			num(&b, cx+r)
			b.WriteByte(' ')
			num(&b, cy-r)
			b.WriteString("l")
			num(&b, -2*r)
			b.WriteByte(' ')
			num(&b, 2*r)
			b.WriteString(`"/>`)
		case f == Open:
			// A ring above the nut.
			b.WriteString(`<circle cx="`)
			num(&b, x(i))
			b.WriteString(`" cy="`)
			num(&b, padTop-7.5)
			b.WriteString(`" r="3" stroke-width="1.1" fill="none"/>`)
		case f >= in.StartFret && f < in.StartFret+in.Window:
			b.WriteString(`<circle cx="`)
			num(&b, x(i))
			b.WriteString(`" cy="`)
			num(&b, yFor(f))
			b.WriteString(`" r="`)
			num(&b, dotRadius)
			b.WriteString(`" stroke="none"/>`)
		}
	}

	// Finger numbers under the diagram, which is what turns a picture of dots
	// into something playable.
	if hasAny(in.Fingers) {
		b.WriteString(`<g font-size="8" text-anchor="middle" stroke="none">`)
		for i, fg := range in.Fingers {
			if fg <= 0 || i >= len(in.Frets) || in.Frets[i] <= 0 {
				continue
			}
			b.WriteString(`<text x="`)
			num(&b, x(i))
			b.WriteString(`" y="`)
			num(&b, padTop+gridH+10)
			b.WriteString(`">`)
			b.WriteString(fingerLabel(fg))
			b.WriteString(`</text>`)
		}
		b.WriteString(`</g>`)
	}

	b.WriteString(`</svg>`)
	_, err := io.WriteString(w, b.String())
	return err
}

// normalize fills in defaults and makes the input self-consistent, so a caller
// cannot produce a diagram that references a fret outside its own window.
func normalize(in Input) Input {
	if in.Strings <= 0 {
		in.Strings = 6
	}
	if len(in.Frets) < in.Strings {
		padded := make([]int, in.Strings)
		copy(padded, in.Frets)
		for i := len(in.Frets); i < in.Strings; i++ {
			padded[i] = Muted
		}
		in.Frets = padded
	}
	in.Frets = in.Frets[:in.Strings]
	if in.Window <= 0 {
		in.Window = defaultWindow
	}

	// Choose the window so every fretted note is inside it. An open-position
	// shape starts at the nut; anything higher starts far enough up to fit.
	lo, hi := 0, 0
	for _, f := range in.Frets {
		if f <= 0 {
			continue
		}
		if lo == 0 || f < lo {
			lo = f
		}
		if f > hi {
			hi = f
		}
	}
	if in.StartFret <= 0 {
		switch {
		case lo == 0:
			in.StartFret = 1
		case hi <= in.Window:
			in.StartFret = 1
		default:
			in.StartFret = lo
		}
	}
	if hi >= in.StartFret+in.Window {
		in.Window = hi - in.StartFret + 1
	}
	return in
}

func hasAny(fingers []int) bool {
	for _, f := range fingers {
		if f > 0 {
			return true
		}
	}
	return false
}

// fingerLabel names a finger. The thumb really is used in some idiomatic shapes,
// so it gets a letter rather than being pretended away.
func fingerLabel(f int) string {
	if f == 5 {
		return "P" // pulgar
	}
	return string(rune('0' + f))
}

// num writes a number with at most one decimal place and no trailing zero,
// which keeps the markup short and the golden files readable.
func num(b *strings.Builder, v float64) {
	if v == float64(int(v)) {
		fmt.Fprintf(b, "%d", int(v))
		return
	}
	fmt.Fprintf(b, "%.1f", v)
}

// escaped returns text safe to place in an SVG text node.
//
// This package's output is treated as markup by the templates that embed it, so
// it is responsible for its own escaping and must not be handed anything it does
// not escape itself.
//
// Characters that XML simply cannot represent are dropped rather than encoded.
// There is no escape for them — a numeric reference to a control character is
// just as illegal — and a stray one in a chord name is not worth making the whole
// document unparseable for.
func escaped(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\'':
			b.WriteString("&#39;")
		case xmlLegal(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// xmlLegal reports whether a rune may appear in an XML 1.0 document at all.
func xmlLegal(r rune) bool {
	switch {
	case r == 0x09, r == 0x0A, r == 0x0D:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}
