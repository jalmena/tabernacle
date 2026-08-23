package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// say renders the tests' own expectations in the base language, which is the
// one their Spanish assertions are written in.
var say = i18n.For(i18n.Spanish)

// corpus is every shape the collection holds for every root of every quality it
// models. The naming rule has to be right about all of them, not about the
// handful that happen to appear in the example songs.
func corpus(t *testing.T) []voicing.Voicing {
	t.Helper()

	roots := []string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}
	var out []voicing.Voicing
	for _, r := range roots {
		for _, q := range harmony.All() {
			c, err := chord.Parse(r + q.Symbol())
			if err != nil {
				continue // a symbol this package cannot spell is not our subject
			}
			out = append(out, voicing.For(c, voicing.Options{Max: 12}).Voicings...)
		}
		// Slash chords too: they are the only shapes with a required bass.
		for _, bass := range []string{"/E", "/G", "/B"} {
			if c, err := chord.Parse(r + bass); err == nil {
				out = append(out, voicing.For(c, voicing.Options{Max: 12}).Voicings...)
			}
		}
	}
	if len(out) < 100 {
		t.Fatalf("the corpus is only %d shapes; this test would prove little", len(out))
	}
	return out
}

var captionNumber = regexp.MustCompile(`(\d+)`)

// The caption and the drawn window are one decision, so a caption can never name
// a fret the picture does not show.
//
// Asserted against the caption's own words rather than against a second
// computation of the rule: a parallel calculation would agree with itself while
// both drifted from what a reader sees.
func TestCaptionAndWindowAgree(t *testing.T) {
	for _, v := range corpus(t) {
		w := windowFor(v)
		name := shapeName(say, v)

		if w.Frets != windowFrets {
			t.Fatalf("%v: the window shows %d frets, want %d", v.Frets, w.Frets, windowFrets)
		}
		if w.Start < 1 {
			t.Fatalf("%v: the window starts at %d", v.Frets, w.Start)
		}

		// Every dot has to be inside its own window, or the diagram package has
		// to widen it behind our back.
		for _, f := range v.Frets {
			if f <= 0 {
				continue
			}
			if int(f) < w.Start || int(f) >= w.Start+w.Frets {
				t.Errorf("%q on %v: fret %d is outside the window [%d, %d)",
					name, v.Frets, f, w.Start, w.Start+w.Frets)
			}
		}

		// Any number in the words has to be a fret the picture contains.
		if m := captionNumber.FindString(name); m != "" {
			n, err := strconv.Atoi(m)
			if err != nil {
				t.Fatalf("%q: unreadable number", name)
			}
			if n < w.Start || n >= w.Start+w.Frets {
				t.Errorf("%q on %v names fret %d, but the window is [%d, %d)",
					name, v.Frets, n, w.Start, w.Start+w.Frets)
			}
		}

		switch {
		case strings.HasPrefix(name, "Cejilla"):
			lowest := int(v.Barres[0].Fret)
			for _, b := range v.Barres {
				if int(b.Fret) < lowest {
					lowest = int(b.Fret)
				}
			}
			// The caption names the barre; the window starts at the hand. On an
			// A-shape barre those are different frets, and both have to be drawn.
			if !strings.HasSuffix(name, strconv.Itoa(lowest)) {
				t.Errorf("%q on %v does not name its barre at %d", name, v.Frets, lowest)
			}
			if w.Start > lowest {
				t.Errorf("%q on %v: the window starts at %d, past the barre at %d",
					name, v.Frets, w.Start, lowest)
			}
		case name == "Posición abierta":
			if w.Start != 1 {
				t.Errorf("%q on %v starts at fret %d", name, v.Frets, w.Start)
			}
			if !v.HasOpenString() {
				t.Errorf("%v is called open position with no open string", v.Frets)
			}
			if v.Reach() > firstPositionFrets {
				t.Errorf("%v reaches fret %d and is still called open position",
					v.Frets, v.Reach())
			}
		case name == "Cuerdas al aire":
			if v.Reach() != 0 {
				t.Errorf("%v is called all-open but frets fret %d", v.Frets, v.Reach())
			}
		}
	}
}

// The chords everybody learns first are open chords, whether or not they happen
// to contain a first-fret note.
func TestOpenPositionIsCalledOpenPosition(t *testing.T) {
	for _, sym := range []string{"G", "G7", "C", "D", "A", "Am", "E", "Em", "D7", "Am7", "Em7", "C7"} {
		t.Run(sym, func(t *testing.T) {
			v := firstShape(t, sym)
			if got := shapeName(say, v); got != "Posición abierta" {
				t.Errorf("%s (%v) is called %q", sym, v.Frets, got)
			}
			if w := windowFor(v); w.Start != 1 {
				t.Errorf("%s is drawn from fret %d", sym, w.Start)
			}
		})
	}
}

// A barre chord is never an open chord, even at the first fret, and it is named
// by the fret the finger lies across.
func TestBarreNamesItsOwnFret(t *testing.T) {
	tests := map[string]int{"F": 1, "Bb": 1, "Bm": 2, "F#m": 2}
	for sym, want := range tests {
		t.Run(sym, func(t *testing.T) {
			v := firstShape(t, sym)
			if !v.HasBarre() {
				t.Skipf("%s's first shape is not barred; nothing to assert here", sym)
			}
			name, w := shapeName(say, v), windowFor(v)
			if name != "Cejilla en el traste "+strconv.Itoa(want) {
				t.Errorf("%s (%v) is called %q, want the barre at %d", sym, v.Frets, name, want)
			}
			if w.Start != v.Position {
				t.Errorf("%s is drawn from fret %d, want the hand's %d", sym, w.Start, v.Position)
			}
		})
	}
}

// A shape with no open strings is closed wherever it sits, including at the first
// fret. Being near the nut is not the same as using it.
func TestAClosedShapeAtTheFirstFretIsNotOpen(t *testing.T) {
	// x x 1 3 4 3 — an Eb with both bass strings muted.
	v := voicing.Voicing{
		Tuning:   voicing.Standard(),
		Frets:    []voicing.Fret{voicing.Muted, voicing.Muted, 1, 3, 4, 3},
		Fingers:  []voicing.Finger{0, 0, 1, 2, 4, 3},
		Position: 1,
	}
	if got := shapeName(say, v); got != "Traste 1" {
		t.Errorf("shapeName = %q, want \"Traste 1\"", got)
	}
	if w := windowFor(v); w.Start != 1 {
		t.Errorf("the window starts at %d, want 1", w.Start)
	}
}

func firstShape(t *testing.T, sym string) voicing.Voicing {
	t.Helper()
	c, err := chord.Parse(sym)
	if err != nil {
		t.Fatalf("chord.Parse(%q): %v", sym, err)
	}
	res := voicing.For(c, voicing.Options{})
	if !res.Found() {
		t.Fatalf("no shape for %s", sym)
	}
	return res.Voicings[0]
}
