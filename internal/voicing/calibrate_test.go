package voicing

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

// How the numbers in hand.go were arrived at.
//
// Not by hand, and not one judgement at a time. This walks every fitted constant
// against the whole of testdata/judgements.json at once and keeps what satisfies
// most of it, which is the discipline the model before this one did not have:
// there is always a weight that saves the newest case and breaks the one before
// it, and twelve terms later nothing can be moved without moving three things.
//
// It is skipped unless FIT=1 because it takes about an hour. It is kept in the
// tree so that the constants can be re-derived rather than believed, and so that
// adding a judgement to the corpus is a thing anybody can follow up on.
//
//	FIT=1 go test ./internal/voicing/ -run TestFitTheHandToWhatThePlayerSaid -v -timeout 180m

type spec struct {
	Chords []struct {
		Symbol, Level string
	} `json:"chords"`
	Bands []struct {
		Grip, Level string
	} `json:"bands"`
	Ordered []struct {
		Easier, Harder string
	} `json:"ordered"`
	Ratios []struct {
		A, B      string
		Times     float64
		Tolerance float64
	} `json:"ratios"`
	Fingering []struct {
		Grip, Fingers string
	} `json:"fingering"`
	Invalid []struct {
		Grip string
	} `json:"invalid"`
}

func spec1() spec {
	raw, _ := os.ReadFile("testdata/judgements.json")
	var s spec
	json.Unmarshal(raw, &s)
	return s
}

func grade(s spec) (int, int, []string) {
	pass, total := 0, 0
	var bad []string
	note := func(ok bool, what string) {
		total++
		if ok {
			pass++
		} else {
			bad = append(bad, what)
		}
	}
	sc := func(g string) Voicing {
		f, _ := ParseGrip(g)
		return Score(Voicing{Tuning: Standard(), Frets: f})
	}
	for _, c := range s.Chords {
		p, err := chord.Parse(c.Symbol)
		if err != nil {
			continue
		}
		g, ok := Generate(p, GenerateOptions{})
		if !ok || len(g.Root.Shapes) == 0 {
			note(false, c.Symbol+" sin forma")
			continue
		}
		v := g.Root.Shapes[0]
		note(v.Level() == levelNamed[c.Level],
			fmt.Sprintf("%s %s(%d) quiere %s", c.Symbol, levelName(v.Level()), v.Strain(), c.Level))
	}
	for _, b := range s.Bands {
		v := sc(b.Grip)
		note(v.Level() == levelNamed[b.Level],
			fmt.Sprintf("%s %s(%d) quiere %s", b.Grip, levelName(v.Level()), v.Strain(), b.Level))
	}
	for _, o := range s.Ordered {
		a, b := sc(o.Easier), sc(o.Harder)
		note(a.Strain() < b.Strain(),
			fmt.Sprintf("%s(%d) < %s(%d)", o.Easier, a.Strain(), o.Harder, b.Strain()))
	}
	for _, r := range s.Ratios {
		a, b := sc(r.A), sc(r.B)
		got := 0.0
		if a.Strain() > 0 {
			got = float64(b.Strain()) / float64(a.Strain())
		}
		note(math.Abs(got-r.Times) <= r.Tolerance,
			fmt.Sprintf("%s/%s = %.2f quiere %.1f", r.B, r.A, got, r.Times))
	}
	for _, f := range s.Fingering {
		v := sc(f.Grip)
		note(fingersOf(v) == f.Fingers,
			fmt.Sprintf("%s dedos %q quiere %q", f.Grip, fingersOf(v), f.Fingers))
	}
	for _, i := range s.Invalid {
		v := sc(i.Grip)
		note(v.Level() == Inhuman,
			fmt.Sprintf("%s %s(%d) quiere inhumana", i.Grip, levelName(v.Level()), v.Strain()))
	}
	return pass, total, bad
}

// penalty is how wrong the whole corpus is, smoothly, so a search has something
// to follow when the pass count is flat.
func penalty(s spec) float64 {
	mid := func(l Level) float64 {
		lo := 0.0
		if l > 0 {
			lo = float64(levels[l-1])
		}
		hi := float64(levels[len(levels)-1]) * 1.4
		if int(l) < len(levels) {
			hi = float64(levels[l])
		}
		return (lo + hi) / 2
	}
	off := func(got int, want Level) float64 {
		lo, hi := 0.0, math.Inf(1)
		if want > 0 {
			lo = float64(levels[want-1])
		}
		if int(want) < len(levels) {
			hi = float64(levels[want])
		}
		switch {
		case float64(got) < lo:
			return (lo - float64(got)) / max(1, mid(want))
		case float64(got) > hi:
			return (float64(got) - hi) / max(1, mid(want))
		}
		return 0
	}
	sc := func(g string) Voicing {
		f, _ := ParseGrip(g)
		return Score(Voicing{Tuning: Standard(), Frets: f})
	}
	total := 0.0
	for _, c := range s.Chords {
		p, err := chord.Parse(c.Symbol)
		if err != nil {
			continue
		}
		g, ok := Generate(p, GenerateOptions{})
		if !ok || len(g.Root.Shapes) == 0 {
			total += 5
			continue
		}
		total += off(g.Root.Shapes[0].Strain(), levelNamed[c.Level])
	}
	for _, b := range s.Bands {
		total += off(sc(b.Grip).Strain(), levelNamed[b.Level])
	}
	for _, o := range s.Ordered {
		a, b := sc(o.Easier), sc(o.Harder)
		total += math.Max(0, float64(a.Strain()-b.Strain()+1)) / 20
	}
	for _, r := range s.Ratios {
		a, b := sc(r.A), sc(r.B)
		got := 0.0
		if a.Strain() > 0 {
			got = float64(b.Strain()) / float64(a.Strain())
		}
		total += math.Max(0, math.Abs(got-r.Times)-r.Tolerance)
	}
	for _, f := range s.Fingering {
		if fingersOf(sc(f.Grip)) != f.Fingers {
			total += 1
		}
	}
	for _, i := range s.Invalid {
		if v := sc(i.Grip); v.Level() != Inhuman {
			total += off(v.Strain(), Inhuman)
		}
	}
	return total
}

type knob struct {
	name string
	at   *float64
	lo   float64
	hi   float64
}

func knobs() []knob {
	return []knob{
		{"stretchCost", &stretchCost, 0.005, 0.4},
		{"curlCost", &curlCost, 0.001, 0.2},
		{"freeFan", &freeFan, 4, 34},
		{"fanCost", &fanCost, 0.005, 0.5},
		{"spreadCost", &spreadCost, 0.002, 0.4},
		{"squeezeCost", &squeezeCost, 0.0005, 0.1},
		{"tendonCost", &tendonCost, 0.001, 0.3},
		{"wristCost", &wristCost, 0.0005, 0.4},
		{"restTurn", &restTurn, 8, 78},
		{"barreCost", &barreCost, 0.5, 60},
		{"barreSpanCost", &barreSpanCost, 0.01, 3},
		{"pressCost", &pressCost, 0.05, 12},
		{"nutCost", &nutCost, 2, 400},
		{"damping", &damping, 0.1, 40},
		{"gain", &gain, 0.05, 12},
		{"weakRing", &weak[2], 0.02, 12},
		{"weakPinky", &weak[3], 0.05, 30},
		{"maxFan", &maxFan, 26, 62},
		{"knuckleGap", &knuckleGap, 17, 25},
		{"reachIndex", &reach[0], 74, 98},
		{"reachMiddle", &reach[1], 82, 106},
		{"reachRing", &reach[2], 78, 101},
		{"reachPinky", &reach[3], 60, 82},
		{"stiffIM", &stiff[0], 0.6, 1.6},
		{"stiffMR", &stiff[1], 0.9, 2.6},
		{"stiffRP", &stiff[2], 1.6, 5.0},
		{"maxReach", &maxReach, 1.02, 1.30},
		{"minReach", &minReach, 0.35, 0.72},
	}
}

func both(s spec) (int, float64, []string) {
	p, n, bad := grade(s)
	_ = n
	return p, penalty(s), bad
}

func TestFitTheHandToWhatThePlayerSaid(t *testing.T) {
	const held = 40 // judgements in the corpus
	if os.Getenv("FIT") == "" {
		t.Skip("set FIT=1 to refit the hand; it takes an hour")
	}
	s := spec1()
	score := func() (int, float64) {
		p, pen, _ := both(s)
		return p, pen
	}
	bestPass, bestPen := score()
	fmt.Printf("start %d/%d  penalty %.2f\n", bestPass, held, bestPen)

	ks := knobs()
	for round := range 9 {
		moved := false
		for _, k := range ks {
			was := *k.at
			for _, f := range []float64{0.6, 0.78, 0.9, 1.12, 1.3, 1.7} {
				v := math.Min(k.hi, math.Max(k.lo, was*f))
				if v == was {
					continue
				}
				*k.at = v
				p, pen := score()
				if p > bestPass || (p == bestPass && pen < bestPen-1e-9) {
					bestPass, bestPen, was, moved = p, pen, v, true
				}
			}
			*k.at = was
		}
		fmt.Printf("round %d: %d/%d  penalty %.2f\n", round+1, bestPass, held, bestPen)
		if !moved {
			break
		}
	}
	fmt.Println("--- what to write into hand.go")
	for _, k := range ks {
		fmt.Printf("%-14s = %.4f\n", k.name, *k.at)
	}
	_, _, bad := grade(s)
	for _, b := range bad {
		fmt.Printf("still owed: %s\n", b)
	}
}
