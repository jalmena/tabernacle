package voicing

import (
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
)

// What one shape costs to judge. The chord page generates hundreds per request,
// so this is the number that decides whether a placement model can be asked for
// on every page load or has to be precomputed.
func BenchmarkPlaceOneShape(b *testing.B) {
	f, _ := ParseGrip("1-3-3-2-1-1")
	v := Score(Voicing{Tuning: Standard(), Frets: f})
	b.ResetTimer()
	for b.Loop() {
		place(v)
	}
}

// What a whole request costs: every shape for one chord, fingered and judged.
func BenchmarkGenerateOneChord(b *testing.B) {
	c, err := chord.Parse("Am7")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		Generate(c, GenerateOptions{})
	}
}

// And the worst case the page can be asked for: every shape of every quality.
func BenchmarkGenerateEveryQuality(b *testing.B) {
	opts := GenerateOptions{Tuning: Standard()}
	qs := harmony.All()
	b.ResetTimer()
	for b.Loop() {
		for _, q := range qs {
			shapesFor(Requirement{Root: 0, Quality: q, Bass: -1}, opts)
		}
	}
}
