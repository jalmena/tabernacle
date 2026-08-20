package chord

import "testing"

func TestFormatSolfege(t *testing.T) {
	tests := map[string]string{
		"C":     "Do",
		"D":     "Re",
		"E":     "Mi",
		"F":     "Fa",
		"G":     "Sol",
		"A":     "La",
		"B":     "Si",
		"Bb":    "Sib",
		"F#m":   "Fa#m",
		"Cmaj7": "Domaj7",
		"C/E":   "Do/Mi",
		"Ab7":   "Lab7",
		"Cm7b5": "Dom7b5",
	}
	for in, want := range tests {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := c.Format(Solfege); got != want {
			t.Errorf("Parse(%q).Format(Solfege) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSolfege(t *testing.T) {
	tests := map[string]string{
		"Do":     "C",
		"Re":     "D",
		"Mi":     "E",
		"Fa":     "F",
		"Sol":    "G",
		"La":     "A",
		"Si":     "B",
		"Sib":    "Bb",
		"Dom":    "Cm",
		"Solm":   "Gm",
		"Mim7":   "Em7",
		"Domaj7": "Cmaj7",
		"Do/Mi":  "C/E",
		"Fa#m":   "F#m",
		"Sib7":   "Bb7",
	}
	for in, want := range tests {
		c, err := ParseIn(Solfege, in)
		if err != nil {
			t.Errorf("ParseIn(Solfege, %q): %v", in, err)
			continue
		}
		if got := c.String(); got != want {
			t.Errorf("ParseIn(Solfege, %q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSolfegeRejectsText(t *testing.T) {
	// The words that would otherwise sneak through: "Solo" starts with Sol,
	// "Mira" with Mi, "Dominante" with Do.
	for _, s := range []string{"Solo", "Mira", "Dominante", "Reggae", "Final", "C", "Am"} {
		if got, err := ParseIn(Solfege, s); err == nil {
			t.Errorf("ParseIn(Solfege, %q) = %v, want error", s, got)
		}
	}
}
