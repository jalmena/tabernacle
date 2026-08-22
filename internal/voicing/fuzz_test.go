package voicing

import (
	"slices"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/harmony"
)

// A grip comes out of a query string, so it is untrusted text.
func FuzzParseGrip(f *testing.F) {
	for _, s := range []string{
		"x-3-2-0-1-0", "0-2-2-1-0-0", "x-x-0-2-3-2", "", "x", "-", "-----",
		"1-2-3-4-5-6-7-8", "x-3-2-0-1-99", "x-3-2-0-1--1", "X-3-2-0-1-0",
		"0-0-0-0-0-0-0-0", "٣-٢-١-٠-x-x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		frets, ok := ParseGrip(s)
		if !ok {
			return
		}
		if len(frets) < 4 || len(frets) > 8 {
			t.Fatalf("ParseGrip(%q) accepted %d strings", s, len(frets))
		}
		for _, x := range frets {
			if x < Muted || x > 24 {
				t.Fatalf("ParseGrip(%q) accepted fret %d", s, x)
			}
		}
		// What it accepts, it writes back the same, or a link would not survive
		// a round trip through the page.
		v := Voicing{Tuning: Standard(), Frets: frets}
		if len(frets) == Standard().Strings() {
			again, ok := ParseGrip(v.Grip())
			if !ok || !slices.Equal(again, frets) {
				t.Fatalf("ParseGrip(%q) -> %v -> %q did not round-trip", s, frets, v.Grip())
			}
		}
		// And it never accepts a spelling with something extra in it.
		if strings.ContainsAny(s, " \t\n") {
			t.Fatalf("ParseGrip(%q) accepted whitespace inside", s)
		}
	})
}

// Identify is fed whatever a reader drew, so it is fed nonsense.
func FuzzIdentify(f *testing.F) {
	for _, s := range []string{
		"x-3-2-0-1-0", "x-x-x-x-x-x", "0-1-2-3-4-5", "12-12-12-12-12-12",
		"x-x-x-x-x-0", "24-24-24-24-24-24",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		frets, ok := ParseGrip(s)
		if !ok {
			return
		}
		v := Voicing{Tuning: Standard(), Frets: frets}
		if len(frets) != Standard().Strings() {
			return
		}
		for _, r := range Identify(frets, Standard()) {
			// Every reading has to be one this package would stand behind: the
			// notes really do make that chord.
			req := Requirement{Root: r.Root, Quality: r.Quality, Bass: -1}
			if ok, why := req.Satisfies(v); !ok {
				t.Fatalf("%s was read as %s, which it does not satisfy: %s", s, r.Chord, why)
			}
			if r.Chord.Root.PitchClass() != r.Root {
				t.Fatalf("%s: named %s but rooted on %d", s, r.Chord, r.Root)
			}
			if r.Inverted != r.Chord.HasBass {
				t.Fatalf("%s: %s says inverted=%v", s, r.Chord, r.Inverted)
			}
		}
	})
}

// Generation runs on demand, for whatever chord the query names, so it has to
// terminate and produce only things a hand can hold.
func FuzzGenerate(f *testing.F) {
	for _, seed := range [][2]int{{0, 1}, {5, 3}, {11, 8}, {3, 19}} {
		f.Add(seed[0], seed[1], -1)
	}
	all := harmony.All()
	f.Fuzz(func(t *testing.T, root, qi, bass int) {
		if root < 0 || root > 11 || qi < 0 || qi >= len(all) || bass < -1 || bass > 11 {
			return
		}
		q := all[qi]
		if q == harmony.Unknown {
			return
		}
		r := Requirement{Root: root, Quality: q, Bass: bass}
		for _, v := range shapesFor(r, GenerateOptions{Tuning: Standard()}) {
			if ok, why := r.Satisfies(v); !ok {
				t.Fatalf("generated %s for %d%s but it does not satisfy it: %s",
					v.Grip(), root, q.Symbol(), why)
			}
			if v.Span() > maxSpan {
				t.Fatalf("generated %s, which spans %d frets", v.Grip(), v.Span())
			}
			if v.Position > maxPosition {
				t.Fatalf("generated %s, at position %d", v.Grip(), v.Position)
			}
			if len(v.Fingers) != len(v.Frets) {
				t.Fatalf("generated %s with %d fingers for %d strings",
					v.Grip(), len(v.Fingers), len(v.Frets))
			}
		}
	})
}
