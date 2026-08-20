package song

import (
	"strings"
	"testing"
)

func TestKindForLabel(t *testing.T) {
	tests := map[string]Kind{
		"Intro":        KindIntro,
		"Verse":        KindVerse,
		"Verse 1":      KindVerse,
		"VERSE II":     KindVerse,
		"verse-2":      KindVerse,
		"Estrofa 2":    KindVerse,
		"Pre-Chorus":   KindPreChorus,
		"pre chorus":   KindPreChorus,
		"Chorus":       KindChorus,
		"Chorus (x2)":  KindChorus,
		"Estribillo":   KindChorus,
		"Coro":         KindChorus,
		"Bridge":       KindBridge,
		"Puente":       KindBridge,
		"Solo":         KindSolo,
		"Instrumental": KindSolo,
		"Outro":        KindOutro,
		"Coda":         KindOutro,
		"Tab":          KindTab,
		"Tablatura":    KindTab,
		"Introducción": KindIntro,
		"Whatever":     KindOther,
		"":             KindOther,
	}
	for label, want := range tests {
		if got := KindForLabel(label); got != want {
			t.Errorf("KindForLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func parseSong(t *testing.T, src string) Song {
	t.Helper()
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

// chordsOf flattens the chord symbols of a song in reading order.
func chordsOf(s Song) []string {
	var out []string
	for _, sec := range s.Sections {
		for _, ln := range sec.Lines {
			for _, tok := range ln.Tokens {
				if tok.Kind == TokenChord {
					out = append(out, tok.Chord.String())
				}
			}
		}
	}
	return out
}

func TestTransposeSong(t *testing.T) {
	src := "---\ntitle: T\nkey: E\n---\n\n## Verse 1\n[E]one [A]two [B]three [C#m]four [A/C#]five [x2]\n"
	s := parseSong(t, src)

	up := s.Transpose(1)
	if up.Key != "F" {
		t.Errorf("key = %q, want %q", up.Key, "F")
	}
	// F major reads in flats, so every chord on the page has to agree.
	want := "F Bb C Dm Bb/D"
	if got := strings.Join(chordsOf(up), " "); got != want {
		t.Errorf("chords = %q, want %q", got, want)
	}
	// The annotation must not have been touched.
	if !strings.Contains(string(mustMarshal(t, up)), "[x2]") {
		t.Error("annotation did not survive transposition")
	}
	// And the original must be untouched.
	if got := strings.Join(chordsOf(s), " "); got != "E A B C#m A/C#" {
		t.Errorf("original mutated: %q", got)
	}
}

func TestTransposeNoKeyUsesFirstChord(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Verse 1\n[Am]one [Dm]two [E]three\n"
	s := parseSong(t, src)

	// Am up three semitones lands in Cm, which is written with flats.
	up := s.Transpose(3)
	if got, want := strings.Join(chordsOf(up), " "), "Cm Fm G"; got != want {
		t.Errorf("chords = %q, want %q", got, want)
	}
	if up.Key != "" {
		t.Errorf("key = %q, want it left empty", up.Key)
	}
}

func TestTransposeIdentity(t *testing.T) {
	src := "---\ntitle: T\nkey: Gb\n---\n\n## Verse 1\n[Gb]one [Db]two [Cb]three\n"
	s := parseSong(t, src)
	for _, n := range []int{0, 12, -12} {
		got := s.Transpose(n)
		if key := got.Key; key != "Gb" {
			t.Errorf("Transpose(%d) key = %q, want Gb", n, key)
		}
		if c := strings.Join(chordsOf(got), " "); c != "Gb Db Cb" {
			t.Errorf("Transpose(%d) chords = %q, want them unchanged", n, c)
		}
	}
}

func TestTransposeLeavesCapoAlone(t *testing.T) {
	src := "---\ntitle: T\nkey: G\ncapo: 2\n---\n\n## Verse 1\n[G]one\n"
	s := parseSong(t, src).Transpose(2)
	if s.Capo != 2 {
		t.Errorf("capo = %d, want 2", s.Capo)
	}
}

func mustMarshal(t *testing.T, s Song) []byte {
	t.Helper()
	b, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}
