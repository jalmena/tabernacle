package voicing

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/harmony"
)

// pcName is for failure messages only; the real spelling lives in chord.
var pcName = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

func label(k key) string {
	s := pcName[k.root] + k.quality.Symbol()
	if k.bass >= 0 {
		s += "/" + pcName[k.bass]
	}
	return s
}

// The generator has to rediscover, on its own, every shape a human typed into
// the dataset by hand.
//
// This is the test that decides whether the rules are right. A curated shape the
// generator cannot find means either a rule that is too strict or a real
// guitarism it does not model, and the exception list is asserted *exact* so it
// cannot grow in silence. Fret vectors are compared, not fingerings: the
// generator's fingerings are valid, the curated ones are idiomatic, and that
// difference is deliberate.
func TestGenerationCoversTheCuratedDataset(t *testing.T) {
	// Shapes the generator is not expected to find, each with the reason.
	// Adding to this list is a decision, not a fix.
	allowedMisses := map[string]string{}

	ds, err := load()
	if err != nil {
		t.Fatal(err)
	}
	opts := GenerateOptions{Tuning: Standard()}

	var missed []string
	total := 0
	for k, shapes := range ds.curated {
		req := Requirement{Root: k.root, Quality: k.quality, Bass: k.bass}
		found := make(map[string]bool)
		for _, v := range shapesFor(req, opts) {
			found[fretKey(v.Frets)] = true
		}
		for _, v := range shapes {
			total++
			if !found[fretKey(v.Frets)] {
				missed = append(missed, fmt.Sprintf("%s %v", label(k), v.Frets))
			}
		}
	}
	if total < 40 {
		t.Fatalf("only %d curated shapes were tried", total)
	}

	sort.Strings(missed)
	for _, m := range missed {
		if _, ok := allowedMisses[m]; !ok {
			t.Errorf("the generator does not find the curated shape %s", m)
		}
	}
	for m := range allowedMisses {
		if !slices.Contains(missed, m) {
			t.Errorf("%s is listed as a known miss but the generator finds it; remove the exception", m)
		}
	}
}

// Every generated shape passes the same checker that validates the hand-written
// dataset. One standard, whoever wrote the shape.
func TestGeneratedShapesAreCorrect(t *testing.T) {
	opts := GenerateOptions{Tuning: Standard()}
	checked := 0
	for _, q := range harmony.All() {
		for _, root := range []int{0, 4, 7, 10} { // C, E, G, A#: open, awkward and barre country
			req := Requirement{Root: root, Quality: q, Bass: -1}
			for _, v := range shapesFor(req, opts) {
				checked++
				checkShape(t, fmt.Sprintf("%s%s %v", pcName[root], q.Symbol(), v.Frets),
					v, root, q, -1)
			}
		}
	}
	if checked < 500 {
		t.Fatalf("only %d generated shapes were checked", checked)
	}
}

// Playability, stated as the limits a hand actually has.
func TestGeneratedShapesArePlayable(t *testing.T) {
	opts := GenerateOptions{Tuning: Standard()}
	for _, q := range harmony.All() {
		for root := range 12 {
			for _, v := range shapesFor(Requirement{Root: root, Quality: q, Bass: -1}, opts) {
				what := fmt.Sprintf("%s%s %v", pcName[root], q.Symbol(), v.Frets)
				if v.Span() > maxSpan {
					t.Errorf("%s: spans %d frets", what, v.Span())
				}
				if mutedInside(v.Frets) > 0 {
					t.Errorf("%s: %d muted strings inside, which nobody is going to damp",
						what, mutedInside(v.Frets))
				}
				// A finger may lie across two strings at the same fret; it may
				// not be on two different frets unless it is the barre.
				on := map[Finger]Fret{}
				for i, f := range v.Fingers {
					if f == Thumb {
						t.Errorf("%s: uses the thumb", what)
					}
					if f <= 0 || v.Frets[i] <= 0 {
						continue
					}
					if prev, ok := on[f]; ok && prev != v.Frets[i] && !barresFinger(v, f) {
						t.Errorf("%s: finger %d is on frets %d and %d at once", what, f, prev, v.Frets[i])
					}
					on[f] = v.Frets[i]
				}
				if len(on) > 4 {
					t.Errorf("%s: needs %d fingers", what, len(on))
				}
			}
		}
	}
}

func barresFinger(v Voicing, f Finger) bool {
	for _, b := range v.Barres {
		if b.Finger == f {
			return true
		}
	}
	return false
}

// The shapes are ordered: what somebody wrote down for this chord first, easiest
// of those first, and then everything else fullest first and easiest within
// that.
func TestTheWrittenShapeLeadsAndTheRestAreFullestFirst(t *testing.T) {
	ds, err := load()
	if err != nil {
		t.Fatal(err)
	}
	opts := GenerateOptions{Tuning: Standard()}
	for _, sym := range []string{"C", "Am7", "F", "G7", "Bm"} {
		req := requirement(t, sym)
		written := make(map[string]bool)
		for _, v := range ds.curated[key{root: req.Root, quality: req.Quality, bass: req.Bass}] {
			written[fretKey(v.Frets)] = true
		}
		if len(written) == 0 {
			t.Fatalf("%s is not in the curated dataset, so this proves nothing", sym)
		}

		got := shapesFor(req, opts)
		if len(got) < 5 {
			t.Fatalf("%s produced only %d shapes", sym, len(got))
		}
		if !written[fretKey(got[0].Frets)] {
			t.Errorf("%s leads with %s, which nobody wrote down", sym, got[0].Grip())
		}

		// The documented sort key, read straight off the list a reader is shown.
		full := fullness(req)
		order := func(v Voicing) []int {
			if written[fretKey(v.Frets)] {
				return []int{0, 0, v.Cost}
			}
			return []int{1, -full(v), v.Cost}
		}
		for i := 1; i < len(got); i++ {
			if slices.Compare(order(got[i-1]), order(got[i])) > 0 {
				t.Fatalf("%s: %s is listed before %s but ranks worse\n  %v\n  %v",
					sym, got[i-1].Grip(), got[i].Grip(), order(got[i-1]), order(got[i]))
			}
		}
		var keys []string
		for _, v := range got {
			keys = append(keys, fretKey(v.Frets))
		}
		if len(slices.Compact(slices.Clone(keys))) != len(keys) {
			t.Errorf("%s: the same fret vector appears twice", sym)
		}
	}
}

// The shapes everyone learns first have to be the shapes that come out first.
func TestTheObviousShapesAreFound(t *testing.T) {
	tests := map[string]string{
		"C":     "x-3-2-0-1-0",
		"A":     "x-0-2-2-2-0",
		"G":     "3-2-0-0-0-3",
		"E":     "0-2-2-1-0-0",
		"D":     "x-x-0-2-3-2",
		"Am":    "x-0-2-2-1-0",
		"Em":    "0-2-2-0-0-0",
		"Dm":    "x-x-0-2-3-1",
		"F":     "1-3-3-2-1-1",
		"G7":    "3-2-0-0-0-1",
		"Am7":   "x-0-2-0-1-0",
		"Cmaj7": "x-3-2-0-0-0",
		"Bm":    "x-2-4-4-3-2",
		"Cadd9": "x-3-2-0-3-0",
	}
	opts := GenerateOptions{Tuning: Standard()}
	for sym, want := range tests {
		t.Run(sym, func(t *testing.T) {
			got := shapesFor(requirement(t, sym), opts)
			var keys []string
			for _, v := range got {
				keys = append(keys, v.Grip())
			}
			if !slices.Contains(keys, want) {
				t.Fatalf("%s: %s is not among the %d shapes found\nfirst few: %s",
					sym, want, len(got), strings.Join(keys[:min(8, len(keys))], "  "))
			}
		})
	}
}
