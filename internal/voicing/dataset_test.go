package voicing

import (
	"fmt"
	"testing"

	"github.com/jalmena/tabernacle/internal/harmony"
)

// pitchClasses returns the pitch classes a shape actually sounds.
func pitchClasses(v Voicing) []int {
	var out []int
	for i, f := range v.Frets {
		if f == Muted {
			continue
		}
		out = append(out, (v.Tuning.Open[i]+int(f))%12)
	}
	return out
}

// lowestSounded returns the pitch class of the lowest note the shape sounds.
func lowestSounded(v Voicing) (int, bool) {
	best, found := 0, false
	for i, f := range v.Frets {
		if f == Muted {
			continue
		}
		p := v.Tuning.Open[i] + int(f)
		if !found || p < best {
			best, found = p, true
		}
	}
	return best % 12, found
}

func contains(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// checkShape applies every structural and harmonic rule to one compiled shape.
//
// This is the test that makes a hand-typed dataset trustworthy. Without it,
// "x 0 2 0 1 0" spelling Am7 is a hope; with it, it is an assertion, and a
// mistyped fret number fails the build instead of teaching somebody a wrong
// chord.
func checkShape(t *testing.T, label string, v Voicing, rootPC int, q harmony.Quality, bassPC int) {
	t.Helper()

	if len(v.Frets) != v.Tuning.Strings() {
		t.Errorf("%s: %d frets for a %d-string tuning", label, len(v.Frets), v.Tuning.Strings())
		return
	}
	if len(v.Fingers) != len(v.Frets) {
		t.Errorf("%s: %d fingers for %d strings", label, len(v.Fingers), len(v.Frets))
		return
	}

	// Structure.
	if s := v.Span(); s > maxSpan {
		t.Errorf("%s: spans %d frets, which no hand is going to do", label, s)
	}
	sounded := 0
	for i, f := range v.Frets {
		switch {
		case f == Muted:
			if v.Fingers[i] != NoFinger {
				t.Errorf("%s: string %d is muted but has a finger on it", label, i)
			}
		case f == Open:
			sounded++
			if v.Fingers[i] != NoFinger {
				t.Errorf("%s: string %d is open but has a finger on it", label, i)
			}
		default:
			sounded++
			if v.Fingers[i] == NoFinger && !barreCovers(v, i) {
				t.Errorf("%s: string %d is fretted with no finger and no barre", label, i)
			}
		}
	}
	if sounded < 2 {
		t.Errorf("%s: only %d strings sound, which is not a chord", label, sounded)
	}

	// A finger can only be in one place, unless it is the barre.
	at := map[Finger]Fret{}
	for i, fg := range v.Fingers {
		if fg == NoFinger || v.Frets[i] <= 0 {
			continue
		}
		if prev, seen := at[fg]; seen && prev != v.Frets[i] {
			if !isBarreFinger(v, fg) {
				t.Errorf("%s: finger %d is on frets %d and %d at once", label, fg, prev, v.Frets[i])
			}
		}
		at[fg] = v.Frets[i]
	}

	// The barre invariant is subtler than it looks. Every string in the span must
	// be at or above the barre fret, and both ends must sit exactly on it —
	// because Bm7 (x 2 4 2 3 2) barres at the second fret with the ring and
	// middle fingers stacked on top of it.
	for _, b := range v.Barres {
		if b.From < 0 || b.To >= len(v.Frets) || b.From >= b.To {
			t.Errorf("%s: barre spans strings %d..%d, which is not a range", label, b.From, b.To)
			continue
		}
		for i := b.From; i <= b.To; i++ {
			if v.Frets[i] == Muted {
				continue
			}
			if v.Frets[i] < b.Fret {
				t.Errorf("%s: string %d is at fret %d, below its barre at %d",
					label, i, v.Frets[i], b.Fret)
			}
		}
		if v.Frets[b.From] != b.Fret && v.Frets[b.From] != Muted {
			t.Errorf("%s: the barre's low end (string %d) is at fret %d, not %d",
				label, b.From, v.Frets[b.From], b.Fret)
		}
		if v.Frets[b.To] != b.Fret && v.Frets[b.To] != Muted {
			t.Errorf("%s: the barre's high end (string %d) is at fret %d, not %d",
				label, b.To, v.Frets[b.To], b.Fret)
		}
	}

	// Harmony: this is what proves the fret numbers were typed correctly.
	tones := q.Intervals()
	allowed := make([]int, 0, len(tones))
	for _, iv := range tones {
		allowed = append(allowed, (rootPC+iv)%12)
	}
	if bassPC >= 0 && !contains(allowed, bassPC) {
		allowed = append(allowed, bassPC)
	}
	got := pitchClasses(v)
	for _, p := range got {
		if !contains(allowed, p) {
			t.Errorf("%s: sounds pitch class %d, which is not in %v (root %d, %v)",
				label, p, allowed, rootPC, q)
		}
	}
	if !contains(got, rootPC) {
		t.Errorf("%s: does not sound its own root (pitch class %d)", label, rootPC)
	}
	// The tone that gives the chord its character must be present, or the shape
	// is a different chord that happens to share notes.
	if len(tones) > 1 {
		defining := (rootPC + tones[1]) % 12
		if !contains(got, defining) {
			t.Errorf("%s: is missing the interval that defines %v", label, q)
		}
	}
	// A seventh chord without its seventh is a triad.
	if len(tones) > 3 {
		seventh := (rootPC + tones[3]) % 12
		if !contains(got, seventh) {
			t.Errorf("%s: is missing the seventh that makes it %v", label, q)
		}
	}
	// The lowest note should be the root, or the declared bass, or another chord
	// tone — an inversion is fine, an unrelated note is not.
	if low, ok := lowestSounded(v); ok {
		want := rootPC
		if bassPC >= 0 {
			want = bassPC
		}
		if low != want && !contains(allowed, low) {
			t.Errorf("%s: lowest note is pitch class %d, want %d", label, low, want)
		}
		if bassPC >= 0 && low != bassPC {
			t.Errorf("%s: is a slash chord but its lowest note is %d, not the bass %d",
				label, low, bassPC)
		}
	}
}

func barreCovers(v Voicing, i int) bool {
	for _, b := range v.Barres {
		if i >= b.From && i <= b.To && v.Frets[i] == b.Fret {
			return true
		}
	}
	return false
}

func isBarreFinger(v Voicing, f Finger) bool {
	for _, b := range v.Barres {
		if b.Finger == f {
			return true
		}
	}
	return false
}

func TestDatasetLoads(t *testing.T) {
	ds, err := loadForTest()
	if err != nil {
		t.Fatalf("the dataset does not load: %v", err)
	}
	if len(ds.curated) == 0 {
		t.Error("no curated shapes were compiled")
	}
	if len(ds.movable) == 0 {
		t.Error("no movable shapes were compiled")
	}
}

func TestCuratedShapesAreCorrect(t *testing.T) {
	ds, err := loadForTest()
	if err != nil {
		t.Fatal(err)
	}
	for k, shapes := range ds.curated {
		for _, v := range shapes {
			label := fmt.Sprintf("curated root=%d %v bass=%d shape=%s", k.root, k.quality, k.bass, v.Shape)
			checkShape(t, label, v, k.root, k.quality, k.bass)
		}
	}
}

// Every movable template must be correct at every root it can reach, which is
// the whole reason a template is worth having.
//
// A template legitimately declines some roots: an E-shape asked for E would have
// to sit at the nut, and open-position shapes are curated rather than derived.
// Whether the collection as a whole covers a root is a separate question, asked
// by TestCoverage.
func TestMovableShapesAreCorrectAtEveryRoot(t *testing.T) {
	ds, err := loadForTest()
	if err != nil {
		t.Fatal(err)
	}
	std := Standard()
	for q, shapes := range ds.movable {
		for _, m := range shapes {
			reached := 0
			for root := range 12 {
				slid := m.slide(root, std)
				if len(slid) > 0 {
					reached++
				}
				for _, v := range slid {
					label := fmt.Sprintf("movable %v/%s root=%d pos=%d", q, m.id, root, v.Position)
					checkShape(t, label, v, root, q, -1)
					// A template must never land on the nut: relative zero means
					// the barre line, and at the nut it would silently become an
					// open string.
					if v.Position < 1 {
						t.Errorf("%s: was emitted at the nut", label)
					}
					if v.Position > maxPosition {
						t.Errorf("%s: was emitted above fret %d", label, maxPosition)
					}
				}
			}
			// A template that reaches almost nothing is not pulling its weight.
			if reached < 9 {
				t.Errorf("movable %v/%s only reaches %d of 12 roots", q, m.id, reached)
			}
		}
	}
}

func TestDatasetQualityNamesAreComplete(t *testing.T) {
	// Every name in the mapping must be a real family, or the dataset can
	// silently attach shapes to nothing.
	for name, q := range qualityNames {
		if !q.Known() {
			t.Errorf("quality name %q maps to an unknown family", name)
		}
	}
	// And every family should be nameable, so a future dataset entry has a key
	// to use.
	named := map[harmony.Quality]bool{}
	for _, q := range qualityNames {
		named[q] = true
	}
	for _, q := range harmony.All() {
		if !named[q] {
			t.Errorf("family %v has no dataset key, so shapes cannot be added for it", q)
		}
	}
}
