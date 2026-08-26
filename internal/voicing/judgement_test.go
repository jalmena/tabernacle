package voicing

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

// The judgements a guitarist gave about shapes on a neck, kept in
// testdata/judgements.json and checked all at once.
//
// All at once is the point. The model this replaced was fitted one judgement at
// a time, and there is always a weight that saves the newest case and breaks the
// one before it; twelve terms later, moving any of them moved three things. A
// judgement here is a fact about a hand and stays true whatever the code does,
// so a model that cannot reproduce it is wrong, and a model that reproduces all
// of it is finished.
type judgements struct {
	Chords []struct {
		Symbol  string `json:"symbol"`
		Level   string `json:"level"`
		Why     string `json:"why"`
		Pending string `json:"pending"`
	} `json:"chords"`
	Bands []struct {
		Grip    string `json:"grip"`
		Level   string `json:"level"`
		Why     string `json:"why"`
		Pending string `json:"pending"`
	} `json:"bands"`
	Ordered []struct {
		Easier  string `json:"easier"`
		Harder  string `json:"harder"`
		Why     string `json:"why"`
		Pending string `json:"pending"`
	} `json:"ordered"`
	Ratios []struct {
		A         string  `json:"a"`
		B         string  `json:"b"`
		Times     float64 `json:"times"`
		Tolerance float64 `json:"tolerance"`
		Why       string  `json:"why"`
		Pending   string  `json:"pending"`
	} `json:"ratios"`
	Fingering []struct {
		Grip    string `json:"grip"`
		Fingers string `json:"fingers"`
		Why     string `json:"why"`
		Pending string `json:"pending"`
	} `json:"fingering"`
	Invalid []struct {
		Grip    string `json:"grip"`
		Why     string `json:"why"`
		Pending string `json:"pending"`
	} `json:"invalid"`
	Invariant struct {
		Known int `json:"known_inversions"`
	} `json:"invariant"`
}

func said(t *testing.T) judgements {
	t.Helper()
	raw, err := os.ReadFile("testdata/judgements.json")
	if err != nil {
		t.Fatalf("the judgements are the specification and they are missing: %v", err)
	}
	var j judgements
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatalf("testdata/judgements.json does not parse: %v", err)
	}
	return j
}

var levelNamed = map[string]Level{
	"trivial": Trivial, "easy": Easy, "moderate": Moderate, "advanced": Advanced,
	"complex": Complex, "extreme": Extreme, "inhuman": Inhuman,
}

func levelName(l Level) string { return strings.TrimPrefix(l.Key(), "level.") }

// drawn scores a grip the way the chord page does when somebody draws one.
func drawn(t *testing.T, grip string) Voicing {
	t.Helper()
	frets, ok := ParseGrip(grip)
	if !ok {
		t.Fatalf("%s: not a grip", grip)
	}
	return Score(Voicing{Tuning: Standard(), Frets: frets})
}

func TestTheChordsLandWhereThePlayerPutThem(t *testing.T) {
	for _, c := range said(t).Chords {
		t.Run(c.Symbol, func(t *testing.T) {
			parsed, err := chord.Parse(c.Symbol)
			if err != nil {
				t.Fatalf("%s does not parse: %v", c.Symbol, err)
			}
			g, ok := Generate(parsed, GenerateOptions{})
			if !ok || len(g.Root.Shapes) == 0 {
				t.Fatalf("%s produced no shape", c.Symbol)
			}
			v := g.Root.Shapes[0]
			got := v.Level()
			pending(t, c.Pending, got == levelNamed[c.Level], fmt.Sprintf(
				"%s (%s) is %s (%d), the player says %s%s",
				c.Symbol, v.Grip(), levelName(got), v.Strain(), c.Level, because(c.Why)))
		})
	}
}

func TestTheDrawnShapesLandInTheirBands(t *testing.T) {
	for _, b := range said(t).Bands {
		t.Run(b.Grip, func(t *testing.T) {
			v := drawn(t, b.Grip)
			got := v.Level()
			pending(t, b.Pending, got == levelNamed[b.Level], fmt.Sprintf(
				"%s is %s (%d), the player says %s%s",
				b.Grip, levelName(got), v.Strain(), b.Level, because(b.Why)))
		})
	}
}

func TestTheOrderIsTheOneThePlayerGave(t *testing.T) {
	for _, o := range said(t).Ordered {
		t.Run(o.Easier+" < "+o.Harder, func(t *testing.T) {
			a, b := drawn(t, o.Easier), drawn(t, o.Harder)
			pending(t, o.Pending, a.Strain() < b.Strain(), fmt.Sprintf(
				"%s scores %d and %s scores %d, which is the wrong way round%s",
				o.Easier, a.Strain(), o.Harder, b.Strain(), because(o.Why)))
		})
	}
}

func TestTheRatiosAreTheOnesThePlayerQuoted(t *testing.T) {
	for _, r := range said(t).Ratios {
		t.Run(r.A+" x"+fmt.Sprintf("%.1f", r.Times), func(t *testing.T) {
			a, b := drawn(t, r.A), drawn(t, r.B)
			if a.Strain() == 0 {
				t.Fatalf("%s scores nothing, so there is no ratio to take", r.A)
			}
			got := float64(b.Strain()) / float64(a.Strain())
			pending(t, r.Pending, math.Abs(got-r.Times) <= r.Tolerance, fmt.Sprintf(
				"%s is %.2f times %s (%d against %d), the player says %.1f%s",
				r.B, got, r.A, b.Strain(), a.Strain(), r.Times, because(r.Why)))
		})
	}
}

func TestTheFingeringIsTheOneThePlayerNamed(t *testing.T) {
	for _, f := range said(t).Fingering {
		t.Run(f.Grip, func(t *testing.T) {
			v := drawn(t, f.Grip)
			got := fingersOf(v)
			pending(t, f.Pending, got == f.Fingers, fmt.Sprintf(
				"%s is held %q, the player holds it %q%s",
				f.Grip, got, f.Fingers, because(f.Why)))
		})
	}
}

// fingersOf writes a fingering the way the corpus writes it: sixth string
// first, a dash for a string nobody holds.
func fingersOf(v Voicing) string {
	out := make([]string, len(v.Frets))
	for i := range v.Frets {
		out[i] = "-"
		if v.Frets[i] > 0 && i < len(v.Fingers) && v.Fingers[i] >= Index && v.Fingers[i] <= Pinky {
			out[i] = fmt.Sprint(int(v.Fingers[i]))
		}
	}
	return strings.Join(out, " ")
}

// A shape the hand cannot be placed on says so with the word it already has.
func TestWhatCannotBeHeldIsInhuman(t *testing.T) {
	for _, i := range said(t).Invalid {
		t.Run(i.Grip, func(t *testing.T) {
			v := drawn(t, i.Grip)
			got := v.Level()
			pending(t, i.Pending, got == Inhuman, fmt.Sprintf(
				"%s is %s (%d), and it cannot be held at all%s",
				i.Grip, levelName(got), v.Strain(), because(i.Why)))
		})
	}
}

// pending reports a judgement the model does not satisfy yet, and turns the
// test around: a listed one is expected to fail, and it is the day it starts
// passing that the test complains, because the entry then has to come out of
// the file. Nothing is quietly written off and nothing quietly stays written
// off, which is the difference between a list of exceptions and a list of debts.
func pending(t *testing.T, note string, met bool, complaint string) {
	t.Helper()
	switch {
	case note == "" && !met:
		t.Error(complaint)
	case note != "" && met:
		t.Errorf("this now holds: take \"pending\" off the entry in "+
			"testdata/judgements.json\n    %s", complaint)
	case note != "":
		t.Logf("still owed — %s\n    %s", note, complaint)
	}
}

func because(why string) string {
	if why == "" {
		return ""
	}
	return " — " + why
}

// Pressing a string down is work, so a shape can never come out easier for
// having one more note in it. Not a judgement about any particular shape — a
// judgement about all of them — and it caught two real inversions in a model
// that satisfied every individual case it was shown.
func TestAddingANoteNeverMakesAShapeEasier(t *testing.T) {
	base := []string{
		"1-3-0-0-0-0", "x-3-2-0-1-0", "0-2-2-1-0-0", "x-x-0-2-3-2",
		"x-0-2-2-1-0", "3-2-0-0-3-3", "x-x-3-2-1-0", "x-0-2-2-2-0",
		"5-x-x-x-x-x", "x-x-5-7-x-x", "1-x-1-1-x-x", "x-2-4-4-3-2",
	}
	inversions := 0
	for _, g := range base {
		frets, ok := ParseGrip(g)
		if !ok {
			t.Fatalf("%s: not a grip", g)
		}
		was := Score(Voicing{Tuning: Standard(), Frets: frets}).Strain()
		for i := range frets {
			if frets[i] > 0 {
				continue // already fretted
			}
			for fret := Fret(1); fret <= 12; fret++ {
				grown := make([]Fret, len(frets))
				copy(grown, frets)
				grown[i] = fret
				v := Voicing{Tuning: Standard(), Frets: grown}
				if v.Span() > maxSpan || mutedInside(grown) > 0 {
					continue // not a shape the model is asked to judge
				}
				if now := Score(v).Strain(); now < was {
					inversions++
					t.Logf("%s scores %d, but %s scores %d with one more note",
						g, was, Voicing{Frets: grown}.Grip(), now)
				}
			}
		}
	}

	switch known := said(t).Invariant.Known; {
	case inversions > known:
		t.Errorf("%d shapes score less than the notes inside them, and only %d "+
			"were owed", inversions, known)
	case inversions < known:
		t.Errorf("only %d inversions left of the %d owed: lower known_inversions "+
			"in testdata/judgements.json", inversions, known)
	}
}
