package voicing

import (
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
)

func parse(t *testing.T, sym string) chord.Chord {
	t.Helper()
	c, err := chord.Parse(sym)
	if err != nil {
		t.Fatalf("chord.Parse(%q): %v", sym, err)
	}
	return c
}

func lookup(t *testing.T, sym string) Result {
	t.Helper()
	return For(parse(t, sym), Options{})
}

// The chords a guitarist meets on an ordinary song sheet must all resolve
// exactly, with no substitution and no warning.
func TestCommonChordsResolveExactly(t *testing.T) {
	symbols := []string{
		"C", "D", "E", "F", "G", "A", "B",
		"Cm", "Dm", "Em", "Fm", "Gm", "Am", "Bm",
		"C7", "D7", "E7", "F7", "G7", "A7", "B7",
		"Cm7", "Dm7", "Em7", "Fm7", "Gm7", "Am7", "Bm7",
		"Cmaj7", "Dmaj7", "Emaj7", "Fmaj7", "Gmaj7", "Amaj7",
		"Bb", "Eb", "Ab", "Db", "F#", "C#", "G#",
		"Bbm", "Ebm", "F#m", "C#m", "G#m", "Bbm7", "F#m7",
		"Csus4", "Dsus4", "Esus4", "Asus4", "Dsus2", "Asus2",
		"C5", "E5", "G5", "A5",
		"Am7b5", "Bm7b5", "Cdim7", "Bdim7",
		"D/F#", "C/E", "C/G", "G/B", "Am/G", "Em/B", "F/C", "D/A",
		"A6", "C6", "Am6",
	}
	for _, sym := range symbols {
		t.Run(sym, func(t *testing.T) {
			res := lookup(t, sym)
			if !res.Found() {
				t.Fatalf("no shape at all for %s", sym)
			}
			if !res.Voicings[0].Approx.Exact() {
				t.Errorf("%s resolved only approximately (%v: %s)",
					sym, res.Voicings[0].Approx.Kind, res.Warning)
			}
			if res.Warning != "" {
				t.Errorf("%s produced a warning: %s", sym, res.Warning)
			}
		})
	}
}

// Every root and every core family must have a real shape, or the library has a
// hole a player will fall into.
func TestCoverage(t *testing.T) {
	roots := []string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}
	suffixes := []string{"", "m", "7", "m7", "maj7", "sus4", "5"}
	for _, r := range roots {
		for _, sfx := range suffixes {
			sym := r + sfx
			res := lookup(t, sym)
			if !res.Found() {
				t.Errorf("%s has no shape", sym)
				continue
			}
			if !res.Voicings[0].Approx.Exact() {
				t.Errorf("%s has only an approximate shape", sym)
			}
		}
	}
}

// Db and C# are the same fingering. The spelling is a presentation choice and
// must never decide which shape a click opens.
func TestEnharmonicSpellingsGiveTheSameShapes(t *testing.T) {
	pairs := [][2]string{
		{"C#m7", "Dbm7"},
		{"F#", "Gb"},
		{"A#m", "Bbm"},
		{"D#7", "Eb7"},
		{"G#maj7", "Abmaj7"},
	}
	for _, p := range pairs {
		a, b := lookup(t, p[0]), lookup(t, p[1])
		if len(a.Voicings) != len(b.Voicings) {
			t.Errorf("%s gave %d shapes and %s gave %d", p[0], len(a.Voicings), p[1], len(b.Voicings))
			continue
		}
		for i := range a.Voicings {
			if fretKey(a.Voicings[i].Frets) != fretKey(b.Voicings[i].Frets) {
				t.Errorf("%s and %s differ at shape %d: %v vs %v",
					p[0], p[1], i, a.Voicings[i].Frets, b.Voicings[i].Frets)
			}
		}
	}
}

// The same chord written in solfege has to reach the same shapes, since the
// notation never reaches this layer.
func TestSolfegeReachesTheSameShapes(t *testing.T) {
	english := lookup(t, "C#m7")
	sol, err := chord.ParseIn(chord.Solfege, "Do#m7")
	if err != nil {
		t.Fatal(err)
	}
	got := For(sol, Options{})
	if len(got.Voicings) != len(english.Voicings) {
		t.Fatalf("solfege gave %d shapes, english gave %d", len(got.Voicings), len(english.Voicings))
	}
	for i := range got.Voicings {
		if fretKey(got.Voicings[i].Frets) != fretKey(english.Voicings[i].Frets) {
			t.Errorf("shape %d differs between notations", i)
		}
	}
}

func TestFallbackChain(t *testing.T) {
	tests := []struct {
		sym      string
		wantKind ApproxKind
		given    string
	}{
		// Decoration this package does not model is dropped, and said so.
		{"Cmaj7#11", QualitySimplified, "Cmaj7"},
		{"C13", QualitySimplified, "C7"},
		{"C7b9", QualitySimplified, "C7"},
		// A family with no shapes walks toward one that has them.
		{"Cmaj9", QualitySimplified, "Cmaj7"},
		{"Cm9", QualitySimplified, "Cm7"},
		{"C9", QualitySimplified, "C7"},
		// Cadd9 has a curated shape of its own, so a root that does not is used
		// here instead.
		{"Eadd9", QualitySimplified, "E"},
		// A slash bass is dropped before the quality is touched, because the
		// bass is usually an inversion somebody else is covering.
		{"Cmaj7/E", BassOmitted, "Cmaj7"},
	}
	for _, tt := range tests {
		t.Run(tt.sym, func(t *testing.T) {
			res := lookup(t, tt.sym)
			if !res.Found() {
				t.Fatalf("%s produced nothing at all", tt.sym)
			}
			a := res.Voicings[0].Approx
			if a.Kind != tt.wantKind {
				t.Errorf("%s: kind = %v, want %v", tt.sym, a.Kind, tt.wantKind)
			}
			if a.Given != tt.given {
				t.Errorf("%s: shows %q, want %q", tt.sym, a.Given, tt.given)
			}
			if res.Warning == "" {
				t.Error("an approximate result must say so")
			}
			if !strings.Contains(res.Warning, tt.given) {
				t.Errorf("the warning %q does not name what is being shown", res.Warning)
			}
		})
	}
}

// "3 de 5" must not mean three real shapes and two invented.
func TestExactAndApproximateAreNeverMixed(t *testing.T) {
	for _, sym := range []string{
		"C", "Am7", "Cmaj7#11", "Cm9", "C13", "Cmaj7/E", "D/F#", "Cadd9",
	} {
		res := lookup(t, sym)
		if len(res.Voicings) == 0 {
			continue
		}
		first := res.Voicings[0].Approx.Kind
		for i, v := range res.Voicings {
			if v.Approx.Kind != first {
				t.Errorf("%s: shape %d is %v but the first is %v", sym, i, v.Approx.Kind, first)
			}
		}
	}
}

// A chord with nothing recognisable must be refused, not guessed at: no diagram
// beats a wrong one.
func TestUnclassifiableChordIsRefused(t *testing.T) {
	c := chord.Chord{Root: chord.Note{Letter: 'C'}, Suffix: "alt"}
	res := For(c, Options{})
	if res.Found() {
		t.Errorf("invented %d shapes for an unclassifiable chord", len(res.Voicings))
	}
	if res.Warning == "" {
		t.Error("a refusal must explain itself")
	}
}

// The ordering contract. These are the chords that expose a bad cost function.
func TestOrdering(t *testing.T) {
	// An open shape always beats a barre.
	for _, sym := range []string{"C", "G", "Am", "Em", "D", "A", "E", "Dm"} {
		res := lookup(t, sym)
		if !res.Found() {
			t.Fatalf("no shapes for %s", sym)
		}
		first := res.Voicings[0]
		if first.HasBarre() {
			t.Errorf("%s offers a barre before its open shape", sym)
		}
		if first.Position > 3 {
			t.Errorf("%s offers a shape at fret %d first", sym, first.Position)
		}
	}

	// Bb: the conventional first choice is the A-shape at the first fret, which
	// is what every chord reference prints, so that is what the ordering pins.
	// A low wide barre carries a cost penalty because the neck is thickest and
	// the tension highest there, but it does not outweigh sitting five frets
	// lower.
	res := lookup(t, "Bb")
	if !res.Found() {
		t.Fatal("no shapes for Bb")
	}
	if got := res.Voicings[0].Position; got != 1 {
		t.Errorf("Bb offers a shape at fret %d first, want the first-fret A shape", got)
	}
	if len(res.Voicings) < 2 {
		t.Error("Bb offers no alternative position")
	}

	// Ordering is by cost and is stable, because the interface says "2 de 4".
	for _, sym := range []string{"F", "Bb", "B", "F#m", "Am7", "C#m7"} {
		res := lookup(t, sym)
		for i := 1; i < len(res.Voicings); i++ {
			if res.Voicings[i].Cost < res.Voicings[i-1].Cost {
				t.Errorf("%s: shape %d costs less than the one before it", sym, i)
			}
		}
	}
}

func TestOrderingIsDeterministic(t *testing.T) {
	for _, sym := range []string{"C", "F", "Bb", "Am7", "C#m7", "G"} {
		first := lookup(t, sym)
		for range 5 {
			again := lookup(t, sym)
			if len(again.Voicings) != len(first.Voicings) {
				t.Fatalf("%s: the number of shapes varies between calls", sym)
			}
			for i := range first.Voicings {
				if first.Voicings[i].ID() != again.Voicings[i].ID() {
					t.Errorf("%s: shape %d is %q one time and %q another",
						sym, i, first.Voicings[i].ID(), again.Voicings[i].ID())
				}
			}
		}
	}
}

func TestShapeIDsAreUniqueWithinAResult(t *testing.T) {
	for _, sym := range []string{"C", "F", "Bb", "Am7", "G", "D", "F#m"} {
		res := lookup(t, sym)
		seen := map[string]bool{}
		for _, v := range res.Voicings {
			if v.ID() == "" {
				t.Errorf("%s: a shape has no id, so it cannot be linked to", sym)
			}
			if seen[v.ID()] {
				t.Errorf("%s: two shapes share the id %q", sym, v.ID())
			}
			seen[v.ID()] = true
		}
	}
}

func TestMaxLimitsTheResult(t *testing.T) {
	res := For(parse(t, "F"), Options{Max: 2})
	if len(res.Voicings) > 2 {
		t.Errorf("got %d shapes with Max 2", len(res.Voicings))
	}
	res = For(parse(t, "F"), Options{})
	if len(res.Voicings) > DefaultMax {
		t.Errorf("got %d shapes with the default limit of %d", len(res.Voicings), DefaultMax)
	}
	if len(res.Voicings) < 2 {
		t.Errorf("F offers only %d shapes; a barre chord should have alternatives", len(res.Voicings))
	}
}

func TestNoResultBreaksThePlayabilityLimits(t *testing.T) {
	for _, sym := range []string{"C", "F", "Bb", "Am7", "Cmaj7", "F#m7", "Ab", "Cdim7", "Am7b5"} {
		for _, v := range lookup(t, sym).Voicings {
			if s := v.Span(); s > maxSpan {
				t.Errorf("%s: a shape spans %d frets", sym, s)
			}
			if v.Position > maxPosition {
				t.Errorf("%s: a shape sits at fret %d", sym, v.Position)
			}
		}
	}
}

func TestParseTuning(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "EADGBE", true},
		{"EADGBE", "EADGBE", true},
		{"eadgbe", "EADGBE", true},
		{"standard", "EADGBE", true},
		{"Drop D", "Drop D", true},
		{"drop d", "Drop D", true},
		{"DROP-D", "Drop D", true},
		{"dropd", "Drop D", true},
		{"DADGAD", "DADGAD", true},
		{"half step down", "Eb standard", true},
		{"Eb standard", "Eb standard", true},
		{"Open G", "Open G", true},
		// The song format promises free text, so an unrecognised value is not an
		// error here; it simply means no diagrams.
		{"Drop D (ish)", "", false},
		{"whatever the guy on the video said", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseTuning(tt.in)
		if ok != tt.ok {
			t.Errorf("ParseTuning(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && got.Name != tt.want {
			t.Errorf("ParseTuning(%q) = %q, want %q", tt.in, got.Name, tt.want)
		}
	}
}

// Drop D differs from standard on the lowest string only, so every shape that
// mutes or omits it is valid verbatim. That turns "no diagrams" into "the shapes
// that actually work" for the commonest non-standard tuning there is.
func TestDropDKeepsTheShapesThatAvoidTheLowString(t *testing.T) {
	dropD, ok := ParseTuning("Drop D")
	if !ok {
		t.Fatal("Drop D is not recognised")
	}
	// Am, C and Am7 all mute the low E, so they survive unchanged.
	for _, sym := range []string{"Am", "C", "Am7", "D", "Dm"} {
		res := For(parse(t, sym), Options{Tuning: dropD})
		if !res.Found() {
			t.Errorf("%s has no shape in Drop D even though it does not use the low string", sym)
		}
	}
	// A shape that does sound the low E must not be offered as if it were right.
	for _, v := range For(parse(t, "G"), Options{Tuning: dropD}).Voicings {
		if v.Frets[0] != Muted {
			// If it sounds the lowest string, the tuning must match.
			if !v.Tuning.Equal(dropD) {
				t.Errorf("a shape sounding the low string was offered for the wrong tuning")
			}
			// And it must still be the right chord: the validator's rules apply.
			pcs := pitchClasses(v)
			if len(pcs) == 0 {
				t.Error("a shape sounds nothing")
			}
		}
	}
}

// A tuning where nothing survives gets an honest warning rather than shapes that
// are quietly wrong.
func TestUnsupportedTuningWarnsInsteadOfLying(t *testing.T) {
	dadgad, ok := ParseTuning("DADGAD")
	if !ok {
		t.Fatal("DADGAD is not recognised")
	}
	res := For(parse(t, "C"), Options{Tuning: dadgad})
	for _, v := range res.Voicings {
		// Anything offered must genuinely sound a C chord in this tuning.
		if !contains(pitchClasses(v), 0) {
			t.Error("a shape offered for DADGAD does not sound the root")
		}
	}
	if !res.Found() && res.Warning == "" {
		t.Error("an empty result for an exotic tuning must explain itself")
	}
	if !res.Found() && !strings.Contains(res.Warning, "DADGAD") {
		t.Errorf("the warning %q should name the tuning", res.Warning)
	}
}

func TestSpecParsers(t *testing.T) {
	frets, err := parseFrets("x 0 2 0 1 0")
	if err != nil {
		t.Fatalf("parseFrets: %v", err)
	}
	want := []Fret{Muted, 0, 2, 0, 1, 0}
	for i := range want {
		if frets[i] != want[i] {
			t.Errorf("parseFrets gave %v, want %v", frets, want)
			break
		}
	}
	// Double figures have to work, which is the whole reason the dataset is
	// written with separators instead of as "x02010".
	if frets, err := parseFrets("x 12 14 12 13 12"); err != nil || frets[2] != 14 {
		t.Errorf("parseFrets on high frets = %v, %v", frets, err)
	}

	for _, bad := range []string{"", "   ", "x 0 y", "x 0 -3", "x 0 99", "x 0 2.5"} {
		if _, err := parseFrets(bad); err == nil {
			t.Errorf("parseFrets(%q) was accepted", bad)
		}
	}

	fingers, err := parseFingers("- - 2 - 1 T", 6)
	if err != nil {
		t.Fatalf("parseFingers: %v", err)
	}
	if fingers[2] != Middle || fingers[5] != Thumb || fingers[0] != NoFinger {
		t.Errorf("parseFingers gave %v", fingers)
	}
	// An absent specification means no fingers, not an error: a diagram without
	// fingering is still a diagram.
	if got, err := parseFingers("", 6); err != nil || len(got) != 6 {
		t.Errorf("parseFingers on empty input = %v, %v", got, err)
	}
	for _, bad := range []string{"- - 2", "- - 9 - 1 -", "- - z - 1 -"} {
		if _, err := parseFingers(bad, 6); err == nil {
			t.Errorf("parseFingers(%q) was accepted", bad)
		}
	}
}

func TestVoicingAccessors(t *testing.T) {
	res := lookup(t, "C")
	v := res.Voicings[0]
	if v.Strings() != 6 {
		t.Errorf("Strings() = %d", v.Strings())
	}
	if v.Span() != 3 {
		t.Errorf("C spans %d frets, want 3", v.Span())
	}
	if v.ID() == "" {
		t.Error("no id")
	}
	if res.Quality != harmony.Major {
		t.Errorf("quality = %v, want Major", res.Quality)
	}
}

// Whatever chord.Parse accepts, a lookup must survive without panicking and
// without returning something that breaks its own rules.
func FuzzFor(f *testing.F) {
	for _, s := range []string{"C", "Am7", "F#m7b5", "Cmaj7#11", "D/F#", "C6/9", "Bb5", "Dno3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sym string) {
		c, err := chord.Parse(sym)
		if err != nil {
			return
		}
		res := For(c, Options{})
		if len(res.Voicings) > DefaultMax {
			t.Fatalf("%q returned %d shapes", sym, len(res.Voicings))
		}
		if !res.Found() && res.Warning == "" {
			t.Fatalf("%q returned nothing and said nothing", sym)
		}
		kinds := map[ApproxKind]bool{}
		for _, v := range res.Voicings {
			if len(v.Frets) != len(v.Fingers) {
				t.Fatalf("%q: %d frets but %d fingers", sym, len(v.Frets), len(v.Fingers))
			}
			if v.Span() > maxSpan {
				t.Fatalf("%q: a shape spans %d frets", sym, v.Span())
			}
			if v.Position > maxPosition {
				t.Fatalf("%q: a shape sits at fret %d", sym, v.Position)
			}
			if v.ID() == "" {
				t.Fatalf("%q: a shape has no id", sym)
			}
			kinds[v.Approx.Kind] = true
		}
		if len(kinds) > 1 {
			t.Fatalf("%q mixed exact and approximate shapes", sym)
		}
	})
}
