package render

import (
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/i18n"
)

func values(cs []Choice) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Value
		if c.On {
			out[i] += "*"
		}
	}
	return strings.Join(out, " ")
}

func TestTheChooserMovesTheWholeChordToAnotherNote(t *testing.T) {
	v := Pick(i18n.English, "Am7")
	if v.Symbol != "Am7" {
		t.Fatalf("read as %q", v.Symbol)
	}
	want := "Cm7 C#m7 Dm7 Ebm7 Em7 Fm7 F#m7 Gm7 G#m7 Am7* Bbm7 Bm7"
	if got := values(v.Roots); got != want {
		t.Errorf("the notes offer\n%s\nwant\n%s", got, want)
	}

	// The bass goes with it, or it would belong to another chord.
	v = Pick(i18n.English, "C/E")
	if got := values(v.Roots); !strings.HasPrefix(got, "C/E* Db/F D/F# ") {
		t.Errorf("a slash chord moves as %s", got)
	}
}

func TestTheChooserKeepsTheNoteAndTheBassWhenTheKindChanges(t *testing.T) {
	v := Pick(i18n.English, "D/F#")
	got := values(v.Qualities)
	if !strings.HasPrefix(got, "D/F#* Dm/F# D7/F# Dmaj7/F# ") {
		t.Errorf("the kinds offer %s", got)
	}
	if v.Qualities[1].Label != "Dm · minor" {
		t.Errorf("the minor one is called %q", v.Qualities[1].Label)
	}
}

// However a chord is spelled, the list has to show which of its kinds it is.
func TestTheChooserKnowsAKindByAnyOfItsSpellings(t *testing.T) {
	for typed, want := range map[string]string{
		"Amin": "Am", "A-": "Am", "AM7": "Amaj7", "A°": "Adim", "Amin7": "Am7",
	} {
		on := ""
		for _, q := range Pick(i18n.English, typed).Qualities {
			if q.On {
				on = q.Value
			}
		}
		if on != want {
			t.Errorf("%s shows as %q, want %q", typed, on, want)
		}
	}
}

// A kind the list has no name for is offered as it is written, so that opening
// the chooser on it does not show it as a major chord.
func TestTheChooserKeepsAKindItHasNoNameFor(t *testing.T) {
	v := Pick(i18n.English, "C13#11")
	if len(v.Qualities) != len(qualityChoices)+1 {
		t.Fatalf("%d kinds on offer, want one more than the %d it names", len(v.Qualities), len(qualityChoices))
	}
	if first := v.Qualities[0]; first.Value != "C13#11" || !first.On || first.Label != "C13#11" {
		t.Errorf("the first kind is %+v", first)
	}
	for _, q := range v.Qualities[1:] {
		if q.On {
			t.Errorf("%s is shown as chosen too", q.Value)
		}
	}
}

func TestTheChooserOffersEveryOtherNoteAsTheBass(t *testing.T) {
	v := Pick(i18n.English, "Am7/G")
	want := "Am7 Am7/A# Am7/B Am7/C Am7/C# Am7/D Am7/D# Am7/E Am7/F Am7/F# Am7/G* Am7/G#"
	if got := values(v.Basses); got != want {
		t.Errorf("the basses offer\n%s\nwant\n%s", got, want)
	}
	if v.Basses[0].Label != "the root" {
		t.Errorf("no bass is called %q", v.Basses[0].Label)
	}

	// Spelled the way the chord is, and the bass that is there as it is written.
	v = Pick(i18n.English, "Eb/Gb")
	if got := values(v.Basses); !strings.Contains(got, " Eb/Gb* ") || !strings.Contains(got, " Eb/Bb ") {
		t.Errorf("a chord in flats offers %s", got)
	}
	if got := values(Pick(i18n.English, "Eb").Basses); !strings.HasPrefix(got, "Eb* Eb/E Eb/F Eb/Gb ") {
		t.Errorf("with no bass it offers %s", got)
	}
}

// What is typed is read generously, and written the way a file writes it.
func TestTheChooserReadsWhatWasTyped(t *testing.T) {
	for typed, want := range map[string]string{"am": "Am", "lam": "Am", "SOL7": "G7", " Bb ": "Bb"} {
		if got := Pick(i18n.Spanish, typed).Symbol; got != want {
			t.Errorf("%q was read as %q, want %q", typed, got, want)
		}
	}
}

// What is not a chord has nothing to choose between, only somewhere to start.
func TestTheChooserStartsFromNothing(t *testing.T) {
	for _, typed := range []string{"", "x2", "let ring"} {
		v := Pick(i18n.English, typed)
		if v.Symbol != "" || len(v.Qualities) != 0 || len(v.Basses) != 0 {
			t.Errorf("%q offers kinds or basses: %+v", typed, v)
		}
		if got := values(v.Roots); got != "C Db D Eb E F F# G Ab A Bb B" {
			t.Errorf("%q offers the notes %s", typed, got)
		}
	}
}

// Every choice is a chord, and the same one whichever way it is then read.
func TestEveryChoiceIsAChordTheFileCanHold(t *testing.T) {
	for _, typed := range []string{"C", "F#m7b5", "Bb/D", "C13#11", "Abmaj7", "E7sus4/B"} {
		v := Pick(i18n.English, typed)
		for _, group := range [][]Choice{v.Roots, v.Qualities, v.Basses} {
			for _, c := range group {
				got, err := chord.Parse(c.Value)
				if err != nil {
					t.Errorf("%s offers %q, which is not a chord", typed, c.Value)
				} else if got.String() != c.Value {
					t.Errorf("%s offers %q, which is written %q", typed, c.Value, got.String())
				}
			}
		}
	}
}
