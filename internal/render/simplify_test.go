package render

import (
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/voicing"
)

const jazzy = `---
title: Jazzy
key: C
---

## Verse 1
[Cmaj7#11]one [Am9]two [D13]three [Gmaj9]four
[Cmaj7]five [Am7]six [D7]seven [G]eight
`

func chordTexts(v SheetView) []string {
	out := make([]string, 0, len(v.Chords))
	for _, c := range v.Chords {
		out = append(out, c.Text)
	}
	return out
}

// Simplifying stops at the first chord the collection can actually play, rather
// than walking all the way down to a triad. Cmaj7#11 becomes Cmaj7, which is
// playable and still sounds like what was written; carrying on to a bare C would
// throw away character for nothing.
func TestSimplifyStopsAtSomethingPlayable(t *testing.T) {
	v := Sheet(parse(t, jazzy), Options{Diagrams: true, Simplify: true})

	got := strings.Join(chordTexts(v), " ")
	if got != "Cmaj7 Am7 D7 Gmaj7 G" {
		t.Errorf("chords = %q, want \"Cmaj7 Am7 D7 Gmaj7 G\"", got)
	}
	if !v.Simplified {
		t.Error("Simplified is false although chords were rewritten")
	}
	if v.SimplifyNote == "" {
		t.Error("no note saying the chords are not what the file says")
	}
	// Every chord shown must now have a shape with nothing given up. That is the
	// whole promise of the control.
	for _, c := range v.Chords {
		if len(c.Voicings) == 0 {
			t.Errorf("%s still has no shape", c.Text)
			continue
		}
		if c.Warning != "" {
			t.Errorf("%s still carries a warning: %s", c.Text, c.Warning)
		}
	}
}

func TestSimplifyLeavesAPlayableSongAlone(t *testing.T) {
	plain := "---\ntitle: T\nkey: G\n---\n\n## Verse 1\n[G]one [C]two [Em]three [D]four\n"
	before := Sheet(parse(t, plain), Options{Diagrams: true})
	after := Sheet(parse(t, plain), Options{Diagrams: true, Simplify: true})

	if strings.Join(chordTexts(after), " ") != strings.Join(chordTexts(before), " ") {
		t.Errorf("chords changed from %v to %v", chordTexts(before), chordTexts(after))
	}
	if after.Simplified {
		t.Error("Simplified is true although nothing needed simplifying")
	}
	if after.SimplifyNote != "" {
		t.Errorf("an unnecessary note: %q", after.SimplifyNote)
	}
}

// The control is only worth offering when pressing it would change something.
func TestCanSimplifyOnlyWhenItWouldDoSomething(t *testing.T) {
	jazz := Sheet(parse(t, jazzy), Options{Diagrams: true})
	if !jazz.CanSimplify {
		t.Error("a song full of unplayable chords does not offer to simplify")
	}

	plain := Sheet(parse(t, "---\ntitle: T\n---\n\n## V\n[G]a [C]b\n"), Options{Diagrams: true})
	if plain.CanSimplify {
		t.Error("a plain song offers to simplify with nothing to do")
	}
	// Already simplified: CanSimplify is not needed, since Simplified carries it.
	on := Sheet(parse(t, jazzy), Options{Diagrams: true, Simplify: true})
	if !on.Simplified {
		t.Error("Simplified should report the rewrite")
	}
}

// Without diagrams there is no notion of "playable", so nothing is rewritten.
func TestSimplifyNeedsDiagrams(t *testing.T) {
	v := Sheet(parse(t, jazzy), Options{Simplify: true})
	if v.Simplified || v.CanSimplify {
		t.Errorf("simplification happened with no shapes to judge against: %+v",
			[]bool{v.Simplified, v.CanSimplify})
	}
	if got := strings.Join(chordTexts(v), " "); !strings.Contains(got, "Cmaj7#11") {
		t.Errorf("chords = %q, want them untouched", got)
	}
}

// A chord nothing can play is left exactly as written. Rewriting it would cost
// the reader the real chord and give nothing back.
func TestSimplifyLeavesAnUnhelpableChordAlone(t *testing.T) {
	// An exotic tuning means no shape matches anything, so no simplification can
	// help and none should be attempted.
	dadgad, ok := voicing.ParseTuning("DADGAD")
	if !ok {
		t.Fatal("DADGAD is not recognised")
	}
	v := Sheet(parse(t, jazzy), Options{Diagrams: true, Simplify: true, Tuning: dadgad})
	if got := strings.Join(chordTexts(v), " "); !strings.Contains(got, "Cmaj7#11") {
		t.Errorf("chords = %q, want them left as written when nothing can help", got)
	}
	if v.Simplified {
		t.Error("Simplified is true although nothing was usefully rewritten")
	}
}

// It composes with transposition, and the words are never touched.
func TestSimplifyComposesWithTransposition(t *testing.T) {
	v := Sheet(parse(t, jazzy), Options{Diagrams: true, Simplify: true, Transpose: 2})
	got := strings.Join(chordTexts(v), " ")
	if !strings.Contains(got, "Dmaj7") {
		t.Errorf("chords = %q, want the simplified chords transposed up two", got)
	}
	if strings.Contains(got, "#11") {
		t.Errorf("chords = %q, want the decoration dropped", got)
	}

	s := parse(t, jazzy)
	if got, want := reconstruct(v), strings.TrimSuffix(s.PlainText(), "\n"); got != want {
		t.Errorf("simplifying changed the words\n got: %q\nwant: %q", got, want)
	}
}

// MapChords must not touch the caller's song, or a simplified view would corrupt
// the one the file was read into.
func TestSimplifyDoesNotMutateTheSource(t *testing.T) {
	s := parse(t, jazzy)
	first := s.Chords()[0].String()

	Sheet(s, Options{Diagrams: true, Simplify: true})

	if got := s.Chords()[0].String(); got != first {
		t.Errorf("the source song changed from %q to %q", first, got)
	}
}

// The sheet must not show a simplified shape until the reader asks for one.
//
// A shape offered before it is asked for would make the control pointless: the
// card would already hold Cmaj7's dots under the Cmaj7#11 heading, and pressing
// Simplificar would change only the wording. On a sheet holding both chords the
// same picture would appear twice under two different names.
func TestUnplayableChordsShowNoShapeUntilAsked(t *testing.T) {
	// A fixture of its own: jazzy deliberately holds each awkward chord and its
	// playable counterpart, so it cannot answer this question.
	src := "---\ntitle: T\nkey: C\n---\n\n## Verse 1\n[Cmaj7#11]uno [Am9]dos [D13]tres [Gmaj9]cuatro\n"
	v := Sheet(parse(t, src), Options{Diagrams: true})

	if len(v.Chords) != 4 {
		t.Fatalf("%d chords, want 4", len(v.Chords))
	}
	for _, c := range v.Chords {
		if len(c.Voicings) > 0 {
			t.Errorf("%s was given %d shapes without being asked", c.Text, len(c.Voicings))
		}
		if c.Warning == "" {
			t.Errorf("%s has no shape and does not say so", c.Text)
		}
		// An empty card is a dead end unless it names the way out.
		if !strings.Contains(c.Warning, "Simplificar") {
			t.Errorf("%s does not point at the control: %q", c.Text, c.Warning)
		}
		// And it is a sentence, not a formatting error.
		if strings.Contains(c.Warning, "%!") {
			t.Errorf("%s warns in broken words: %q", c.Text, c.Warning)
		}
	}
}

// Two chords that simplify to the same thing must not have been showing the same
// diagram beforehand. Written against the shapes rather than the labels, because
// identical dots under different headings is what the reader actually saw.
func TestDifferentChordsNeverShareADiagram(t *testing.T) {
	both := "---\ntitle: T\nkey: C\n---\n\n## Verse 1\n[Cmaj7#11]uno [Cmaj7]dos\n"
	v := Sheet(parse(t, both), Options{Diagrams: true})

	seen := map[string]string{}
	for _, c := range v.Chords {
		for _, vo := range c.Voicings {
			if prev, dup := seen[vo.Fingering]; dup {
				t.Errorf("%s and %s are drawn identically", prev, c.Text)
			}
			seen[vo.Fingering] = c.Text
		}
	}
}

// The bass is the last thing given up, and only on request.
//
// A chord keeps its slash bass until the reader presses the control, and then
// loses the bass before the quality, because on a guitar sheet an inversion is
// usually somebody else's note while the quality is what the chord is.
func TestSimplifyDropsTheBassBeforeTheQuality(t *testing.T) {
	src := "---\ntitle: T\nkey: C\n---\n\n## Verse 1\n[Cmaj7/E]uno\n"

	if got := chordTexts(Sheet(parse(t, src), Options{Diagrams: true})); got[0] != "Cmaj7/E" {
		t.Errorf("the chord was rewritten to %q without being asked", got[0])
	}
	got := chordTexts(Sheet(parse(t, src), Options{Diagrams: true, Simplify: true}))
	if got[0] != "Cmaj7" {
		t.Errorf("simplified to %q, want Cmaj7: the seventh is the chord's character "+
			"and the bass is not", got[0])
	}
}

// candidates orders the substitutions it will try, and the order is the whole
// design: most faithful first, so the walk stops at the least change that works.
func TestCandidateOrder(t *testing.T) {
	got := candidates(parseChord(t, "Cmaj9/E"))
	var names []string
	for _, c := range got {
		names = append(names, c.String())
	}
	want := "Cmaj9/E Cmaj9 Cmaj7/E Cmaj7 C/E C"
	if strings.Join(names, " ") != want {
		t.Errorf("candidates = %q, want %q", strings.Join(names, " "), want)
	}
}

func parseChord(t *testing.T, sym string) chord.Chord {
	t.Helper()
	c, err := chord.Parse(sym)
	if err != nil {
		t.Fatalf("chord.Parse(%q): %v", sym, err)
	}
	return c
}
