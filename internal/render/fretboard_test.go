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

// The notes the browser is handed for the shape on the neck.
//
// This is the whole reason the pitches are computed here rather than in
// JavaScript: which note a fret on a string makes is music, so it is worked out
// once, in Go, where it can be checked against a guitar.
func TestTheBoardShipsTheNotesItSounds(t *testing.T) {
	for _, tc := range []struct {
		name, grip, want string
	}{
		// Standard tuning is E2 A2 D3 G3 B3 E4 = 40 45 50 55 59 64.
		{"open strings", "0-0-0-0-0-0", "40,45,50,55,59,64"},
		{"E major", "0-2-2-1-0-0", "40,47,52,56,59,64"},
		// The muted sixth string contributes nothing at all.
		{"C major", "x-3-2-0-1-0", "48,52,55,60,64"},
		// Two muted strings, and the rest still in order.
		{"D major", "x-x-0-2-3-2", "50,57,62,66"},
		// A barre is not a special case: it is six fretted strings.
		{"F major", "1-3-3-2-1-1", "41,48,53,57,60,65"},
		{"nothing sounds", "x-x-x-x-x-x", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := Lab(LabOptions{Grip: tc.grip, Lang: i18n.Spanish, Notation: chord.English})
			if got := v.Board.Pitches; got != tc.want {
				t.Errorf("%s sounds %q, want %q", tc.grip, got, tc.want)
			}
		})
	}
}

// An octave up is twelve more of everything, which is the one arithmetic fact
// the browser relies on when it turns these numbers into a frequency.
func TestTwelveFretsUpIsTwelveHigher(t *testing.T) {
	low := Lab(LabOptions{Grip: "1-3-3-2-1-1", Lang: i18n.Spanish, Notation: chord.English})
	high := Lab(LabOptions{Grip: "13-15-15-14-13-13", Start: 13, Lang: i18n.Spanish, Notation: chord.English})

	lows := strings.Split(low.Board.Pitches, ",")
	highs := strings.Split(high.Board.Pitches, ",")
	if len(lows) != len(highs) {
		t.Fatalf("%d notes against %d", len(lows), len(highs))
	}
	for i := range lows {
		a, _ := strconv.Atoi(lows[i])
		b, _ := strconv.Atoi(highs[i])
		if b-a != 12 {
			t.Errorf("string %d: %d against %d, which is %d apart and not an octave", i, a, b, b-a)
		}
	}
}

// The neck says which finger goes on each dot.
//
// It is the same fingering the rest of the collection is held by, worked out in
// Go rather than left to the reader: a neck that shows dots and says nothing
// about how to hold them is a puzzle rather than a lesson.
func TestTheNeckNamesItsFingers(t *testing.T) {
	v := Lab(LabOptions{Grip: "1-3-3-2-1-1", Lang: i18n.Spanish, Notation: chord.English})

	// F: the index lies across the first fret, the middle takes the second, and
	// the ring and little finger the third.
	want := map[int]int{0: 1, 1: 3, 2: 4, 3: 2, 4: 1, 5: 1}
	for _, row := range v.Board.Strings {
		for _, cell := range row.Cells {
			if !cell.On {
				if cell.Finger != 0 {
					t.Errorf("string %d fret %d is not pressed but names finger %d",
						row.Index, cell.Fret, cell.Finger)
				}
				continue
			}
			if got := cell.Finger; got != want[row.Index] {
				t.Errorf("string %d is held by finger %d, want %d", row.Index, got, want[row.Index])
			}
			if !strings.Contains(cell.Label, "Traste") {
				t.Errorf("string %d says %q, which does not name the fret", row.Index, cell.Label)
			}
		}
	}

	// An empty neck has nothing to say about fingers.
	empty := Lab(LabOptions{Grip: "0-0-0-0-0-0", Lang: i18n.Spanish, Notation: chord.English})
	for _, row := range empty.Board.Strings {
		for _, cell := range row.Cells {
			if cell.Finger != 0 {
				t.Errorf("an open string names finger %d", cell.Finger)
			}
		}
	}
}

// The hand drawn over the neck is the hand the difficulty was worked out with,
// and it has to line up with the grid it is laid on.
func TestTheDrawnHandLandsOnTheBoard(t *testing.T) {
	for _, tc := range []struct{ what, grip string }{
		{"a barre", "1-3-3-2-1-1"},
		{"a diagonal", "x-3-2-1-x-x"},
		{"three fingers across one fret", "x-0-2-2-2-0"},
		{"one finger", "x-x-x-x-x-3"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			frets, ok := voicing.ParseGrip(tc.grip)
			if !ok {
				t.Fatalf("%s: not a grip", tc.grip)
			}
			b := boardFor(say, voicing.Voicing{Tuning: voicing.Standard(), Frets: frets}, 1, LabOptions{})
			if b.Hand == nil {
				t.Fatal("no hand was drawn for a shape a hand can hold")
			}
			held := 0
			for _, f := range b.Hand.Fingers {
				held++
				if f.Number < 1 || f.Number > 4 {
					t.Errorf("finger %d is not one of the four", f.Number)
				}
				// Tips land on a string, and the strings run down the middles of
				// the columns: a half across for the sixth and five and a half
				// for the first. Knuckles sit past the first, which is the side
				// the hand comes over.
				if f.TipX < 0.49 || f.TipX > 5.51 {
					t.Errorf("finger %d holds a note at %.2f, off the strings", f.Number, f.TipX)
				}
				if f.TipY < 0 || f.TipY > float64(b.Hand.Rows) {
					t.Errorf("finger %d holds a note at row %.2f, outside the window", f.Number, f.TipY)
				}
				// The hand comes over the neck from the first string, so its
				// knuckles are past that side and never on the strings.
				if f.KnuckleX <= 5.5 {
					t.Errorf("finger %d has its knuckle at %.2f, on the strings", f.Number, f.KnuckleX)
				}
			}
			if held == 0 {
				t.Error("a hand with no fingers on it")
			}
		})
	}
}

// Nothing fretted is nothing to draw.
func TestNoHandIsDrawnForAnEmptyNeck(t *testing.T) {
	frets, _ := voicing.ParseGrip("0-0-0-0-0-0")
	if b := boardFor(say, voicing.Voicing{Tuning: voicing.Standard(), Frets: frets}, 1, LabOptions{}); b.Hand != nil {
		t.Error("a hand was drawn over six open strings")
	}
}

// The hand hangs outside the frets on purpose — a palm is behind the neck, not
// on it — but not so far that it leaves the card it is drawn on.
func TestTheDrawnHandStaysOnTheCard(t *testing.T) {
	for _, grip := range []string{
		"x-3-2-0-1-0", "x-x-0-2-3-2", "0-2-2-1-0-0", "1-3-3-2-1-1",
		"x-0-2-2-2-0", "3-2-0-0-0-3", "x-2-4-4-4-2", "1-5-1-0-0-0",
	} {
		frets, ok := voicing.ParseGrip(grip)
		if !ok {
			t.Fatalf("%s: not a grip", grip)
		}
		b := boardFor(say, voicing.Voicing{Tuning: voicing.Standard(), Frets: frets}, 1, LabOptions{})
		if b.Hand == nil {
			continue
		}
		for _, f := range b.Hand.Fingers {
			if f.KnuckleY < -0.5 || f.KnuckleY > float64(b.Hand.Rows)+0.5 {
				t.Errorf("%s: finger %d has its knuckle at row %.2f, off the card",
					grip, f.Number, f.KnuckleY)
			}
		}
	}
}

// The drawn fingers must not cross, because two crossing lines say the hand is
// doing something it is not.
//
// They did. The knuckles were squashed onto a line while the rest of each
// knuckle's position was kept, which shortened every finger by a different
// amount and made two of them cross in the picture when they do not cross on
// the neck. A guitarist saw it and said the model should be penalising that,
// which it would have been right to do if it had been true.
func TestTheDrawnFingersDoNotCross(t *testing.T) {
	grips := []string{
		"2-0-0-0-0-1", "x-3-2-0-1-0", "x-x-0-2-3-2", "0-2-2-1-0-0",
		"1-3-3-2-1-1", "x-0-2-2-2-0", "3-2-0-0-0-3", "x-2-4-4-4-2",
		"x-x-3-2-1-0", "x-x-0-2-3-1", "1-3-1-0-0-0", "5-x-x-7-x-x",
		"x-5-7-7-6-5", "0-0-0-2-3-2", "3-x-0-0-0-3", "x-x-5-7-x-x",
	}
	for _, grip := range grips {
		frets, ok := voicing.ParseGrip(grip)
		if !ok {
			t.Fatalf("%s: not a grip", grip)
		}
		b := boardFor(say, voicing.Voicing{Tuning: voicing.Standard(), Frets: frets}, 1, LabOptions{})
		if b.Hand == nil {
			continue
		}
		for i, a := range b.Hand.Fingers {
			for _, c := range b.Hand.Fingers[i+1:] {
				if cross(a, c) {
					t.Errorf("%s: the lines for fingers %d and %d cross",
						grip, a.Number, c.Number)
				}
			}
		}
	}
}

// cross reports whether two drawn fingers meet anywhere but their ends.
func cross(a, b HandFinger) bool {
	d := func(px, py, qx, qy, rx, ry float64) float64 {
		return (qx-px)*(ry-py) - (qy-py)*(rx-px)
	}
	side := func(px, py, qx, qy, rx, ry, sx, sy float64) bool {
		u := d(px, py, qx, qy, rx, ry)
		v := d(px, py, qx, qy, sx, sy)
		return (u > 0 && v < 0) || (u < 0 && v > 0)
	}
	return side(a.KnuckleX, a.KnuckleY, a.TipX, a.TipY, b.KnuckleX, b.KnuckleY, b.TipX, b.TipY) &&
		side(b.KnuckleX, b.KnuckleY, b.TipX, b.TipY, a.KnuckleX, a.KnuckleY, a.TipX, a.TipY)
}

// A kind the chooser offers has to be one the page can then show: each is asked
// for by the symbol it carries, and a symbol nobody can read is a dead end with
// the reader's own choice in it.
func TestEveryKindTheChooserOffersIsAChord(t *testing.T) {
	for _, q := range Lab(LabOptions{Symbol: "C"}).Qualities {
		if v := Lab(LabOptions{Symbol: q.Value}); v.Problem != "" || v.Chord == "" {
			t.Errorf("choosing %q answers %q", q.Value, v.Problem)
		}
	}
}
