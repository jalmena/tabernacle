package voicing

import (
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
)

func chords(t *testing.T, symbols ...string) []chord.Chord {
	t.Helper()
	var out []chord.Chord
	for _, s := range symbols {
		c, err := chord.Parse(s)
		if err != nil {
			t.Fatalf("parsing %q: %v", s, err)
		}
		out = append(out, c)
	}
	return out
}

func symbolsOf(cs []chord.Chord) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.String())
	}
	return strings.Join(out, " ")
}

func optionAt(opts []CapoOption, fret int) CapoOption {
	for _, o := range opts {
		if o.Fret == fret {
			return o
		}
	}
	return CapoOption{Fret: -1}
}

// The arithmetic, on its own, before any judgement about what is easy.
//
// Sound = shape + capo. Holding the sound still and moving the capo up one fret
// means fingering one semitone lower, which is the only claim the rest of the
// file rests on.
func TestMovingTheCapoUpFingersLower(t *testing.T) {
	key := chords(t, "F")[0]
	opts := CapoOptions(key, chords(t, "F", "Bb", "C7"), 0, Options{})

	for _, tc := range []struct {
		fret int
		want string
	}{
		{0, "F Bb C7"},
		{1, "E A B7"},
		{3, "D G A7"},
		{5, "C F G7"},
	} {
		if got := symbolsOf(optionAt(opts, tc.fret).Chords); got != tc.want {
			t.Errorf("capo %d: fingering %q, want %q", tc.fret, got, tc.want)
		}
	}
}

// A song already written for a capo is measured from where the capo is, not
// from the nut: leaving it where it is must not rewrite a single chord.
func TestASongThatAlreadyHasACapoStaysPutAtItsOwnFret(t *testing.T) {
	key := chords(t, "G")[0]
	opts := CapoOptions(key, chords(t, "G", "C", "D"), 2, Options{})

	if got, want := symbolsOf(optionAt(opts, 2).Chords), "G C D"; got != want {
		t.Errorf("capo 2 on a song already at 2: %q, want %q", got, want)
	}
	// One fret higher than it sits is still one semitone lower to finger.
	if got, want := symbolsOf(optionAt(opts, 3).Chords), "F# B C#"; got != want {
		t.Errorf("capo 3: %q, want %q", got, want)
	}
}

// The point of the whole feature. A song in F is the standard example: a
// teacher's answer is capo 1 and play in E, capo 3 and play in D, or capo 5 and
// play in C, and one minor chord stays barred in all three. Getting rid of
// every barre is not the promise — being easier than the sheet you were handed
// is.
func TestABarredSongIsOfferedAWayOutOfTheBarres(t *testing.T) {
	key := chords(t, "F")[0]
	opts := CapoOptions(key, chords(t, "F", "Bb", "C7", "Dm"), 0, Options{})
	plain := optionAt(opts, 0)

	best := opts[0]
	if best.Fret == 0 {
		t.Fatalf("the easiest option is still no capo: %+v", best)
	}
	if best.Score >= plain.Score {
		t.Errorf("capo %d scored %d, no better than no capo at %d", best.Fret, best.Score, plain.Score)
	}
	if best.Barred >= plain.Barred {
		t.Errorf("capo %d barres %d chords, against %d with none", best.Fret, best.Barred, plain.Barred)
	}

	// The three a teacher names, in the order a player would try them.
	for i, want := range []struct {
		fret   int
		chords string
	}{
		{1, "E A B7 C#m"},
		{3, "D G A7 Bm"},
		{5, "C F G7 Am"},
	} {
		got := opts[i]
		if got.Fret != want.fret || symbolsOf(got.Chords) != want.chords {
			t.Errorf("option %d is capo %d (%s), want capo %d (%s)",
				i+1, got.Fret, symbolsOf(got.Chords), want.fret, want.chords)
		}
	}
}

// And the converse, which is what stops the advice being noise: a song that is
// already open chords has nothing to gain, so no capo must win.
func TestAnOpenSongIsLeftAlone(t *testing.T) {
	key := chords(t, "G")[0]
	opts := CapoOptions(key, chords(t, "G", "C", "D", "Em"), 0, Options{})

	if best := opts[0]; best.Fret != 0 {
		t.Errorf("capo %d beat no capo on a song of open chords: %s", best.Fret, symbolsOf(best.Chords))
	}
	if plain := optionAt(opts, 0); plain.Open != 4 || plain.Barred != 0 {
		t.Errorf("G C D Em counted %d open and %d barred, want 4 and 0", plain.Open, plain.Barred)
	}
}

// Nine G's and one F is an easy song with one hard chord in it, not a song with
// nine easy problems.
func TestARepeatedChordIsOneChord(t *testing.T) {
	key := chords(t, "G")[0]
	many := CapoOptions(key, chords(t, "G", "G", "G", "C", "G"), 0, Options{})
	once := CapoOptions(key, chords(t, "G", "C"), 0, Options{})

	if a, b := optionAt(many, 0).Score, optionAt(once, 0).Score; a != b {
		t.Errorf("repeats scored %d against %d for the same two chords", a, b)
	}
}

// Every fret is offered so a caller can compare against doing nothing, and the
// spelling comes from the key rather than chord by chord.
func TestEveryFretIsOfferedAndSpelledByTheKey(t *testing.T) {
	key := chords(t, "Eb")[0]
	opts := CapoOptions(key, chords(t, "Eb", "Ab", "Bb"), 0, Options{})

	if len(opts) != maxCapo+1 {
		t.Fatalf("got %d options, want one per fret from 0 to %d", len(opts), maxCapo)
	}
	// Down a semitone from Eb is D, and D major is spelled sharp: the third
	// chord has to read A, and G, not their flat-side twins.
	if got, want := symbolsOf(optionAt(opts, 1).Chords), "D G A"; got != want {
		t.Errorf("capo 1 out of Eb: %q, want %q", got, want)
	}
	if got := optionAt(opts, 1).Key.String(); got != "D" {
		t.Errorf("capo 1 plays in %q, want D", got)
	}
}

func TestNoChordsIsNoAdvice(t *testing.T) {
	if got := CapoOptions(chords(t, "C")[0], nil, 0, Options{}); got != nil {
		t.Errorf("got %d options for a song with no chords", len(got))
	}
}
