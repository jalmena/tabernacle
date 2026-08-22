package voicing

import (
	"slices"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

func gripOf(t *testing.T, s string) Voicing {
	t.Helper()
	frets, ok := ParseGrip(s)
	if !ok {
		t.Fatalf("ParseGrip(%q) refused it", s)
	}
	return Voicing{Tuning: Standard(), Frets: frets}
}

func requirement(t *testing.T, sym string) Requirement {
	t.Helper()
	c, err := chord.Parse(sym)
	if err != nil {
		t.Fatalf("chord.Parse(%q): %v", sym, err)
	}
	r, ok := RequirementFor(c)
	if !ok {
		t.Fatalf("%s does not classify exactly", sym)
	}
	return r
}

func TestGripRoundTrip(t *testing.T) {
	for _, s := range []string{"x-3-2-0-1-0", "0-2-2-1-0-0", "x-x-0-2-3-2", "3-2-0-0-0-3"} {
		if got := gripOf(t, s).Grip(); got != s {
			t.Errorf("Grip(ParseGrip(%q)) = %q", s, got)
		}
	}
}

func TestParseGripRefusesRubbish(t *testing.T) {
	for _, s := range []string{
		"", "x", "x-x", "x-x-x", "hello", "1-2-3-4-5-6-7-8-9",
		"x-3-2-0-1-99", "x-3-2-0-1--1", "x-3-2-0-1-o", "x 3 2 0 1 0",
		" x-3-2-0-1-0", "x-3-2-0-1-0 ", "x-3-2-0-1-0\n",
	} {
		if _, ok := ParseGrip(s); ok {
			t.Errorf("ParseGrip(%q) accepted it", s)
		}
	}
}

// The omission rule, stated once and checked against every family this package
// models, so adding a twenty-first forces a decision rather than inheriting one.
func TestWhichNotesAChordCanDoWithout(t *testing.T) {
	tests := map[string]struct{ required, optional []int }{
		// A triad keeps everything: three notes is already the minimum.
		"C":     {required: []int{0, 4, 7}},
		"Cm":    {required: []int{0, 3, 7}},
		"C5":    {required: []int{0, 7}},
		"Csus4": {required: []int{0, 5, 7}},
		"Caug":  {required: []int{0, 4, 8}},
		"Cdim":  {required: []int{0, 3, 6}},

		// A seventh and up may drop the fifth, and only the fifth.
		"C7":    {required: []int{0, 4, 10}, optional: []int{7}},
		"Cmaj7": {required: []int{0, 4, 11}, optional: []int{7}},
		"Cm7":   {required: []int{0, 3, 10}, optional: []int{7}},
		"C6":    {required: []int{0, 4, 9}, optional: []int{7}},
		"C9":    {required: []int{0, 2, 4, 10}, optional: []int{7}},

		// An altered fifth is not a perfect fifth, so it stays.
		"Cdim7": {required: []int{0, 3, 6, 9}},
		"Cm7b5": {required: []int{0, 3, 6, 10}},
	}
	for sym, want := range tests {
		t.Run(sym, func(t *testing.T) {
			r := requirement(t, sym)
			got := r.Required()
			for _, pc := range want.required {
				if !slices.Contains(got, pc) {
					t.Errorf("%s does not require %d", sym, pc)
				}
			}
			for _, pc := range want.optional {
				if slices.Contains(got, pc) {
					t.Errorf("%s requires %d, which it should be able to drop", sym, pc)
				}
				if !slices.Contains(r.Allowed(), pc) {
					t.Errorf("%s does not allow %d at all", sym, pc)
				}
			}
		})
	}
}

// Two notes are not a chord, whatever the two are.
func TestTwoNotesAreNotAChord(t *testing.T) {
	// C and E: a third, and every bit of C major that is not the fifth.
	shape := gripOf(t, "x-3-2-x-x-x")
	if ok, _ := requirement(t, "C").Satisfies(shape); ok {
		t.Error("a root and a third were accepted as C major")
	}
	// But a power chord really is two notes.
	if ok, why := requirement(t, "C5").Satisfies(gripOf(t, "x-3-5-5-x-x")); !ok {
		t.Errorf("a power chord was refused: %s", why)
	}
}

// A ninth is a ninth because it is above the root.
func TestAnAddedNinthHasToBeAboveTheRoot(t *testing.T) {
	// Cadd9 as everybody plays it: x 3 2 0 3 0. The D on the second string is
	// well above the C.
	if ok, why := requirement(t, "Cadd9").Satisfies(gripOf(t, "x-3-2-0-3-0")); !ok {
		t.Errorf("the ordinary Cadd9 was refused: %s", why)
	}
	// The same notes with the D underneath is a different chord.
	below := Voicing{Tuning: Standard(), Frets: []Fret{Muted, 5, 2, 0, 1, 0}}
	if ok, _ := requirement(t, "Cadd9").Satisfies(below); ok {
		t.Error("a D below the root was accepted as an added ninth")
	}
}

// A slash chord means what it says about the lowest note.
func TestASlashBassHasToBeLowest(t *testing.T) {
	if ok, why := requirement(t, "C/E").Satisfies(gripOf(t, "0-3-2-0-1-0")); !ok {
		t.Errorf("C/E was refused: %s", why)
	}
	if ok, _ := requirement(t, "C/E").Satisfies(gripOf(t, "x-3-2-0-1-0")); ok {
		t.Error("plain C was accepted as C/E")
	}
}

// A rootless voicing is real practice and cannot honestly be captioned with the
// chord's name.
func TestTheRootHasToSound(t *testing.T) {
	// E, G and B flat: C7 with the C taken out.
	rootless := gripOf(t, "x-7-5-3-x-x")
	if ok, why := requirement(t, "C7").Satisfies(rootless); ok {
		t.Error("a rootless voicing was accepted as C7")
	} else if why == "" {
		t.Error("a refusal with no reason")
	}
}

// The dataset's own checker and this rule have to agree, because the whole point
// of one rule is that there is one.
func TestTheCuratedDatasetSatisfiesItsOwnRequirements(t *testing.T) {
	ds, err := load()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for k, shapes := range ds.curated {
		req := Requirement{Root: k.root, Quality: k.quality, Bass: k.bass}
		for _, v := range shapes {
			checked++
			if ok, why := req.Satisfies(v); !ok {
				t.Errorf("%v (%s) does not satisfy its own chord: %s",
					v.Frets, k.quality.Symbol(), why)
			}
		}
	}
	if checked < 40 {
		t.Fatalf("only %d curated shapes were checked", checked)
	}
}

// And every movable template, at every root it can be slid to.
func TestEveryMovableShapeSatisfiesItsRequirement(t *testing.T) {
	ds, err := load()
	if err != nil {
		t.Fatal(err)
	}
	std := Standard()
	checked := 0
	for q, templates := range ds.movable {
		for root := range 12 {
			req := Requirement{Root: root, Quality: q, Bass: -1}
			for _, m := range templates {
				for _, v := range m.slideQuality(root, period(q), std) {
					checked++
					if ok, why := req.Satisfies(v); !ok {
						t.Errorf("%s at root %d, %v: %s", q.Symbol(), root, v.Frets, why)
					}
				}
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d movable shapes were checked", checked)
	}
}
