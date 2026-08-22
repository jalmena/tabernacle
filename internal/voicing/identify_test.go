package voicing

import (
	"slices"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/harmony"
)

func names(rs []Reading) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Chord.String())
	}
	return out
}

// Adding a twenty-first quality has to be a decision about where it ranks, not
// an accident of map order.
func TestEveryQualityHasAPlaceInThePreferenceOrder(t *testing.T) {
	all := harmony.All()
	for _, q := range all {
		if q == harmony.Unknown {
			continue
		}
		if _, ok := rank[q]; !ok {
			t.Errorf("%s is not in the preference order", q.Symbol())
		}
	}
	if len(preference) != len(all) {
		t.Errorf("the preference order has %d qualities and there are %d", len(preference), len(all))
	}
	if len(slices.Compact(slices.Clone(preference))) != len(preference) {
		t.Error("a quality appears twice in the preference order")
	}
}

func TestIdentifyNamesTheOrdinaryShapes(t *testing.T) {
	tests := map[string]string{
		"x-3-2-0-1-0": "C",
		"0-2-2-1-0-0": "E",
		"x-0-2-2-1-0": "Am",
		"3-2-0-0-0-1": "G7",
		"x-x-0-2-3-1": "Dm",
		"1-3-3-2-1-1": "F",
		"x-0-2-0-1-0": "Am7",
		"x-3-2-0-0-0": "Cmaj7",
		"0-3-2-0-1-0": "C/E",
	}
	for grip, want := range tests {
		t.Run(grip, func(t *testing.T) {
			got := Identify(gripOf(t, grip).Frets, Standard())
			if len(got) == 0 {
				t.Fatalf("%s was not identified at all", grip)
			}
			if got[0].Chord.String() != want {
				t.Errorf("%s read as %s, wanted %s (all: %s)",
					grip, got[0].Chord, want, strings.Join(names(got), ", "))
			}
		})
	}
}

// The ambiguities are the interesting part, so they are asserted rather than
// tolerated.
func TestIdentifyReportsRealAmbiguity(t *testing.T) {
	tests := []struct {
		grip  string
		first string
		also  []string
	}{
		// C E G A: whichever note is underneath decides the name.
		{"x-3-2-2-1-3", "C6", []string{"Am7/C"}},
		{"x-0-2-2-1-3", "Am7", []string{"C6/A"}},
		// Take the G away and there is no sixth left to argue about: three
		// notes, one chord, and the C is simply not its root.
		{"x-3-2-2-1-0", "Am/C", nil},
		// A diminished seventh is four chords at once.
		{"x-3-4-2-4-2", "Cdim7", []string{"Ebdim7/C", "F#dim7/C", "Adim7/C"}},
		// C F G.
		{"x-3-3-0-1-1", "Csus4", []string{"Fsus2/C"}},
	}
	for _, tc := range tests {
		t.Run(tc.grip, func(t *testing.T) {
			got := names(Identify(gripOf(t, tc.grip).Frets, Standard()))
			if len(got) == 0 || got[0] != tc.first {
				t.Fatalf("%s read as %v, wanted %s first", tc.grip, got, tc.first)
			}
			for _, w := range tc.also {
				if !slices.Contains(got, w) {
					t.Errorf("%s: %s is missing from %v", tc.grip, w, got)
				}
			}
		})
	}
}

// A name is never invented.
func TestIdentifyRefusesWhatIsNotAChord(t *testing.T) {
	for _, grip := range []string{
		"x-x-x-x-x-x", // nothing
		"x-x-x-x-x-0", // one note
		"1-2-3-x-x-x", // a chromatic cluster
		"0-1-2-x-x-x",
	} {
		if got := Identify(gripOf(t, grip).Frets, Standard()); len(got) != 0 {
			t.Errorf("%s was read as %v", grip, names(got))
		}
	}
}

// Everything the generator produces for a chord identifies as that chord, and
// leads with it when nothing was left out and the root is underneath.
func TestIdentifyRoundTrip(t *testing.T) {
	opts := GenerateOptions{Tuning: Standard()}
	for _, sym := range []string{"C", "Am", "G7", "Fmaj7", "Bm7", "D6", "Ebdim7", "Aadd9"} {
		t.Run(sym, func(t *testing.T) {
			req := requirement(t, sym)
			for _, v := range shapesFor(req, opts) {
				got := Identify(v.Frets, Standard())
				hit := -1
				for i, r := range got {
					if r.Root == req.Root && r.Quality == req.Quality {
						hit = i
						break
					}
				}
				if hit < 0 {
					t.Fatalf("%s %s did not identify as %s (got %v)", sym, v.Grip(), sym, names(got))
				}
				b, _ := v.Bass()
				if hit != 0 && len(got[hit].Omitted) == 0 && b%12 == req.Root {
					t.Errorf("%s %s: complete and in root position but read as %s first",
						sym, v.Grip(), got[0].Chord)
				}
			}
		})
	}
}
