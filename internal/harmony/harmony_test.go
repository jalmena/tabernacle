package harmony

import (
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

func mustParse(t *testing.T, sym string) chord.Chord {
	t.Helper()
	c, err := chord.Parse(sym)
	if err != nil {
		t.Fatalf("chord.Parse(%q): %v", sym, err)
	}
	return c
}

func TestOfClassifiesExactly(t *testing.T) {
	tests := []struct {
		sym  string
		want Quality
	}{
		{"C", Major},
		{"Cmaj", Major},
		{"CM", Major},
		{"Cm", Minor},
		{"Cmin", Minor},
		{"C-", Minor},
		{"C7", Dominant7},
		{"Cmaj7", Major7},
		{"CM7", Major7},
		{"CΔ7", Major7},
		{"Cm7", Minor7},
		{"Cmin7", Minor7},
		{"C-7", Minor7},
		{"Cm(maj7)", MinorMajor7},
		{"Cmmaj7", MinorMajor7},
		{"Cdim", Diminished},
		{"C°", Diminished},
		{"Cdim7", Diminished7},
		{"Cm7b5", HalfDiminished},
		{"Cm7♭5", HalfDiminished},
		{"Cø7", HalfDiminished},
		{"Caug", Augmented},
		{"C+", Augmented},
		{"Csus2", Sus2},
		{"Csus4", Sus4},
		{"Csus", Sus4},
		{"C7sus4", Dominant7Sus4},
		{"C6", Major6},
		{"Cm6", Minor6},
		{"Cadd9", Add9},
		{"C9", Dominant9},
		{"Cmaj9", Major9},
		{"Cm9", Minor9},
		{"C5", Power},
		{"Cno3", Power},
		// A slash bass does not change the quality.
		{"C/E", Major},
		{"Am7/G", Minor7},
	}
	for _, tt := range tests {
		q, red, ok := Of(mustParse(t, tt.sym))
		if !ok {
			t.Errorf("Of(%q) could not classify it", tt.sym)
			continue
		}
		if q != tt.want {
			t.Errorf("Of(%q) = %v, want %v", tt.sym, q, tt.want)
		}
		if !red.Exact() {
			t.Errorf("Of(%q) dropped %q, but this symbol is understood exactly", tt.sym, red.Dropped)
		}
	}
}

// The bug that otherwise ships: uppercase M is major, lowercase m is minor.
func TestCaseIsMeaningful(t *testing.T) {
	maj, _, _ := Of(mustParse(t, "CM7"))
	min, _, _ := Of(mustParse(t, "Cm7"))
	if maj != Major7 {
		t.Errorf("CM7 = %v, want Major7", maj)
	}
	if min != Minor7 {
		t.Errorf("Cm7 = %v, want Minor7", min)
	}
	if maj == min {
		t.Fatal("CM7 and Cm7 classified the same")
	}
}

func TestOfReducesUnmodelledDecoration(t *testing.T) {
	tests := []struct {
		sym     string
		want    Quality
		dropped string
	}{
		{"Cmaj7#11", Major7, "#11"},
		{"C7b9", Dominant7, "b9"},
		{"C7(b9)", Dominant7, "(b9)"},
		{"C13", Dominant7, "13"},
		{"C11", Dominant7, "11"},
		{"Cm7add11", Minor7, "add11"},
		{"C7#5", Dominant7, "#5"},
		{"Cmaj7(#11)", Major7, "(#11)"},
	}
	for _, tt := range tests {
		q, red, ok := Of(mustParse(t, tt.sym))
		if !ok {
			t.Errorf("Of(%q) could not classify it", tt.sym)
			continue
		}
		if q != tt.want {
			t.Errorf("Of(%q) = %v, want %v", tt.sym, q, tt.want)
		}
		if red.Exact() {
			t.Errorf("Of(%q) reported an exact match, but %q had to be dropped", tt.sym, tt.dropped)
		}
	}
}

// Every family must reach Major or Minor in a few steps, with no cycles. Walking
// the whole enum means a new family with a missing parent fails the build rather
// than looping at runtime.
func TestParentChainsTerminate(t *testing.T) {
	for _, q := range All() {
		steps := 0
		cur := q
		seen := map[Quality]bool{cur: true}
		for {
			next, ok := cur.Parent()
			if !ok {
				break
			}
			if seen[next] {
				t.Fatalf("%v: the simplification chain loops at %v", q, next)
			}
			seen[next] = true
			cur = next
			steps++
			if steps > 6 {
				t.Fatalf("%v: the simplification chain does not terminate", q)
			}
		}
		if cur != Major && cur != Minor {
			t.Errorf("%v simplifies down to %v, want Major or Minor", q, cur)
		}
	}
}

func TestIntervals(t *testing.T) {
	tests := []struct {
		q    Quality
		want []int
	}{
		{Major, []int{0, 4, 7}},
		{Minor, []int{0, 3, 7}},
		{Dominant7, []int{0, 4, 7, 10}},
		{Major7, []int{0, 4, 7, 11}},
		{Minor7, []int{0, 3, 7, 10}},
		{HalfDiminished, []int{0, 3, 6, 10}},
		{Diminished7, []int{0, 3, 6, 9}},
		{Power, []int{0, 7}},
	}
	for _, tt := range tests {
		got := tt.q.Intervals()
		if len(got) != len(tt.want) {
			t.Errorf("%v intervals = %v, want %v", tt.q, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%v intervals = %v, want %v", tt.q, got, tt.want)
				break
			}
		}
	}
	// Every family contains its own root, and the slice is a copy so a caller
	// cannot corrupt the table.
	for _, q := range All() {
		iv := q.Intervals()
		if len(iv) == 0 || iv[0] != 0 {
			t.Errorf("%v does not contain its root: %v", q, iv)
		}
		iv[0] = 99
		if q.Intervals()[0] != 0 {
			t.Fatalf("%v: Intervals returned the table itself, not a copy", q)
		}
	}
}

func TestEveryQualityHasCompleteMetadata(t *testing.T) {
	for _, q := range All() {
		if !q.Known() {
			t.Errorf("%v is in All but reports Known() false", q)
		}
		if len(q.Intervals()) < 2 {
			t.Errorf("%v has too few intervals to be a chord", q)
		}
	}
	// Major is the only family written with an empty suffix.
	empty := 0
	for _, q := range All() {
		if q.Symbol() == "" {
			empty++
		}
	}
	if empty != 1 {
		t.Errorf("%d qualities have an empty symbol, want exactly one (major)", empty)
	}
}

// Every alias must land on a family that exists, and must round-trip: the
// canonical symbol of the family it names has to classify back to it.
func TestAliasesAreConsistent(t *testing.T) {
	for suffix, q := range aliases {
		if !q.Known() {
			t.Errorf("alias %q maps to an unknown quality", suffix)
		}
	}
	for _, q := range All() {
		sym := "C" + q.Symbol()
		c, err := chord.Parse(sym)
		if err != nil {
			t.Errorf("the canonical symbol for %v is %q, which chord.Parse rejects: %v", q, sym, err)
			continue
		}
		got, red, ok := Of(c)
		if !ok || got != q {
			t.Errorf("%v writes itself as %q, which classifies as %v (ok=%v)", q, sym, got, ok)
		}
		if !red.Exact() {
			t.Errorf("%v writes itself as %q, which does not classify exactly", q, sym)
		}
	}
}

func TestOfRefusesNonsense(t *testing.T) {
	// A suffix built only from pieces this package does not model must be
	// refused rather than guessed at, because the caller must not invent a shape.
	for _, suffix := range []string{"alt", "b13alt"} {
		c := chord.Chord{Root: chord.Note{Letter: 'C'}, Suffix: suffix}
		if q, _, ok := Of(c); ok {
			t.Errorf("Of(C%s) classified as %v, want a refusal", suffix, q)
		}
	}
}

func TestSimplify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Unmodelled decoration is dropped first.
		{"Cmaj7#11", "Cmaj7"},
		{"C7b9", "C7"},
		{"C13", "C7"},
		// Then the family walks toward something with shapes.
		{"Cmaj9", "Cmaj7"},
		{"Cm9", "Cm7"},
		{"C9", "C7"},
		{"Cadd9", "C"},
		{"C6", "C"},
		{"Cm6", "Cm"},
		{"Csus4", "C"},
		{"C7sus4", "C7"},
		{"Cdim7", "Cdim"},
		{"Cm7b5", "Cm7"},
		{"C7", "C"},
		{"Cm7", "Cm"},
		{"Cmaj7", "C"},
		// A slash bass rides along; dropping it is the caller's decision.
		{"Cmaj9/E", "Cmaj7/E"},
	}
	for _, tt := range tests {
		got, changed := Simplify(mustParse(t, tt.in))
		if !changed {
			t.Errorf("Simplify(%q) reported no change, want %q", tt.in, tt.want)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("Simplify(%q) = %q, want %q", tt.in, got.String(), tt.want)
		}
	}
	// A plain major triad is already as simple as it gets.
	if _, changed := Simplify(mustParse(t, "C")); changed {
		t.Error("Simplify(C) claimed to have simplified a major triad")
	}
	if _, changed := Simplify(mustParse(t, "Cm")); changed {
		t.Error("Simplify(Cm) claimed to have simplified a minor triad")
	}
}

// Simplifying repeatedly must always come to rest, since the interface offers it
// as a single switch over a whole sheet.
func TestSimplifyTerminates(t *testing.T) {
	for _, sym := range []string{"Cmaj7#11", "Cm9", "C13", "Cdim7", "Cm7b5", "C7sus4"} {
		c := mustParse(t, sym)
		for i := 0; ; i++ {
			next, changed := Simplify(c)
			if !changed {
				break
			}
			c = next
			if i > 8 {
				t.Fatalf("Simplify(%q) does not settle: reached %q", sym, c)
			}
		}
		if s := c.String(); s != "C" && s != "Cm" {
			t.Errorf("%q simplifies down to %q, want C or Cm", sym, s)
		}
	}
}

// Whatever chord.Parse accepts, this package must handle without panicking, and
// must never claim an exact match for something it had to reduce.
func FuzzOf(f *testing.F) {
	for _, s := range []string{"C", "Cm7", "Cmaj7#11", "C7(b9)", "Cø7", "C6/9", "Dno3", "Cm7♭5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sym string) {
		c, err := chord.Parse(sym)
		if err != nil {
			return
		}
		q, red, ok := Of(c)
		if !ok {
			return
		}
		if !q.Known() {
			t.Fatalf("Of(%q) returned an unknown quality", sym)
		}
		// Whatever family was chosen, its own canonical spelling must be a chord
		// symbol, or the simplification chain could produce something unparseable.
		if _, err := chord.Parse("C" + q.Symbol()); err != nil {
			t.Fatalf("%v spells itself %q, which is not a chord symbol", q, q.Symbol())
		}
		if !red.Exact() && red.Dropped == "" {
			t.Fatal("a reduction reported itself inexact but dropped nothing")
		}
		// Simplification must terminate from anywhere.
		cur := c
		for i := 0; ; i++ {
			next, changed := Simplify(cur)
			if !changed {
				break
			}
			cur = next
			if i > 8 {
				t.Fatalf("Simplify(%q) does not settle", sym)
			}
		}
	})
}
