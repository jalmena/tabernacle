package voicing

import (
	"math"
	"sort"
	"testing"

	"github.com/jalmena/tabernacle/internal/harmony"
)

// The quick search against a careful one.
//
// place tries twenty-three angles and looks closely near the promising ones,
// and each angle starts the palm where the last finished. cold walks every half
// degree from a cold start every time. The first is what ships and the second is
// what it has to be nearly as good as.
//
// Nearly, and the test says how nearly rather than pretending it is exact. Half
// the shapes come out identical and ninety-nine in a hundred within about one
// per cent; a handful of high-neck shapes nobody plays come out further out, and
// the numbers here are what stops that getting quietly worse.
//
// The refusals go the other way and are left alone on purpose. Where the careful
// search finds a placement the quick one does not, it is a placement that exists
// inside a window narrower than a degree of wrist — which is not a way of
// holding a chord, it is an artefact of looking hard enough.
func cold(v Voicing) (float64, bool) {
	tips, n, extra, ok := tipsOf(v)
	if !ok || n == 0 {
		if ok {
			return extra, true
		}
		return 0, false
	}
	best, found := math.Inf(1), false
	for deg := turnFrom; deg <= turnTo; deg += 0.5 {
		var lay laying
		lay.load(tips, n)
		if c, ok := lay.costAt(deg); ok && c < best {
			best, found = c, true
		}
	}
	if !found {
		return 0, false
	}
	return best + extra, true
}

func TestTheQuickSearchIsNearlyTheCarefulOne(t *testing.T) {
	var over []float64
	worst, worstAt := 0.0, ""
	checked, knife := 0, 0
	opts := GenerateOptions{Tuning: Standard()}
	for _, q := range harmony.All() {
		for _, root := range []int{0, 3, 7, 10} {
			for _, v := range shapesFor(Requirement{Root: root, Quality: q, Bass: -1}, opts) {
				a, okA := place(v)
				b, okB := cold(v)
				switch {
				case okA && !okB:
					t.Errorf("%s: the quick search holds a shape the careful one "+
						"cannot, which is the wrong way round", v.Grip())
					continue
				case !okA && okB:
					knife++
					continue
				case !okA:
					continue
				}
				checked++
				if b > 0 {
					d := (a - b) / b
					over = append(over, d)
					if d > worst {
						worst, worstAt = d, v.Grip()
					}
				}
			}
		}
	}
	if checked < 500 {
		t.Fatalf("only %d shapes checked", checked)
	}
	sort.Float64s(over)
	p90, p99 := over[len(over)*9/10], over[len(over)*99/100]
	t.Logf("%d shapes: p90 %.2f%%, p99 %.2f%%, worst %.1f%% on %s, %d knife-edge",
		checked, p90*100, p99*100, worst*100, worstAt, knife)
	if p90 > 0.01 {
		t.Errorf("nine shapes in ten should be within one per cent; the ninetieth "+
			"is %.2f%% over", p90*100)
	}
	if p99 > 0.03 {
		t.Errorf("ninety-nine in a hundred should be within three per cent; the "+
			"ninety-ninth is %.2f%% over", p99*100)
	}
	// Not the single worst, which is one shape out of ten thousand and says
	// nothing about the page: how many are badly out is the number that matters.
	bad := 0
	for _, o := range over {
		if o > 0.10 {
			bad++
		}
	}
	if bad > 40 {
		t.Errorf("%d shapes come out more than a tenth over the careful search, "+
			"which was under forty (worst %.0f%% on %s)", bad, worst*100, worstAt)
	}
	if knife > 20 {
		t.Errorf("%d shapes can only be held inside a window narrower than the "+
			"quick search looks, which was twelve", knife)
	}
}
