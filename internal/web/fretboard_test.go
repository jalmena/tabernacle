package web

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/render"
	"github.com/jalmena/tabernacle/internal/song"
)

var (
	// The grid is the one path drawn with no fill; its horizontal commands are the
	// fret lines, one more than the number of frets.
	gridPath   = regexp.MustCompile(`<path stroke-width="0\.7" fill="none" d="([^"]*)"`)
	startLabel = regexp.MustCompile(`text-anchor="end" stroke="none">(\d+)</text>`)
	nutLine    = regexp.MustCompile(`<path stroke-width="3"`)
	capNumber  = regexp.MustCompile(`(\d+)`)
)

// The caption is written by internal/render and the picture by internal/diagram,
// which know nothing of each other. Their agreement is therefore checked here,
// where they meet: on the finished SVG.
func TestTheDiagramMatchesItsCaption(t *testing.T) {
	src := "---\ntitle: T\nartist: A\nkey: C\n---\n\n## Verse 1\n" +
		"[C]a [G]b [D]c [F]d [Bb]e [Bm]f [F#m]g [Am7]h [E]i [Eb]j\n"
	s, err := song.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	sheet := render.Sheet(s, render.Options{Diagrams: true, Notation: chord.English})

	checked := 0
	for _, ref := range sheet.Chords {
		for _, v := range ref.Voicings {
			svg := string(chordDiagram(v, ref.Text))
			checked++

			// The window the caption was derived from is the window that got drawn.
			m := gridPath.FindStringSubmatch(svg)
			if m == nil {
				t.Fatalf("%s: no grid in the diagram", ref.Text)
			}
			if lines := strings.Count(m[1], "H"); lines != v.FretsShown+1 {
				t.Errorf("%s %q: the grid has %d fret lines, want %d for %d frets",
					ref.Text, v.Name, lines, v.FretsShown+1, v.FretsShown)
			}

			// A shape at the nut shows the nut; one further up says where it is.
			label := startLabel.FindStringSubmatch(svg)
			switch {
			case v.StartFret <= 1:
				if !nutLine.MatchString(svg) {
					t.Errorf("%s %q starts at the nut but draws none", ref.Text, v.Name)
				}
				if label != nil {
					t.Errorf("%s %q draws the nut and also labels fret %s",
						ref.Text, v.Name, label[1])
				}
			default:
				if label == nil {
					t.Errorf("%s %q starts at fret %d and says so nowhere",
						ref.Text, v.Name, v.StartFret)
					continue
				}
				if label[1] != strconv.Itoa(v.StartFret) {
					t.Errorf("%s %q: the picture is labelled %s, the window starts at %d",
						ref.Text, v.Name, label[1], v.StartFret)
				}
			}

			// And whatever fret the words name is a fret the picture contains.
			if n := capNumber.FindString(v.Name); n != "" {
				fret, err := strconv.Atoi(n)
				if err != nil {
					t.Fatalf("unreadable caption %q", v.Name)
				}
				if fret < v.StartFret || fret >= v.StartFret+v.FretsShown {
					t.Errorf("%s: %q names fret %d, outside the drawn [%d, %d)",
						ref.Text, v.Name, fret, v.StartFret, v.StartFret+v.FretsShown)
				}
			}
		}
	}
	if checked < 20 {
		t.Fatalf("only %d diagrams were checked; this proves little", checked)
	}
}
