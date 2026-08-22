package i18n

import (
	"strings"
	"testing"
)

// Every language has to say everything, or a reader who chose one gets a page
// half in another. The fallback exists so that never looks broken; this is what
// stops it becoming the way translations are done.
func TestEveryLanguageSaysEverything(t *testing.T) {
	base := Keys()
	if len(base) < 50 {
		t.Fatalf("the base language has %d messages; this test proves little", len(base))
	}
	for _, l := range All() {
		if l == Spanish {
			continue
		}
		for _, key := range base {
			if _, ok := catalogue[l][key]; !ok {
				t.Errorf("%s has no %q", l, key)
			}
		}
		for key := range catalogue[l] {
			if !Has(key) {
				t.Errorf("%s has %q, which the base language does not", l, key)
			}
		}
	}
}

// A message with a placeholder has to have the same ones in every language, or
// formatting it produces "%!d(MISSING)" on somebody's page.
func TestPlaceholdersMatchAcrossLanguages(t *testing.T) {
	for _, key := range Keys() {
		want := strings.Count(catalogue[Spanish][key], "%")
		for _, l := range All() {
			if got := strings.Count(catalogue[l][key], "%"); got != want {
				t.Errorf("%q takes %d arguments in %s and %d in %s", key, want, Spanish, got, l)
			}
		}
	}
}

// A translation that is the same string as the base is usually a key somebody
// copied and forgot. Not always — "Tabernacle" is "Tabernacle" — so this reports
// rather than fails, and the exceptions are named.
func TestTranslationsAreNotCopies(t *testing.T) {
	same := map[string]bool{
		// Words English and Spanish genuinely share here.
		"settings.theme": true,
	}
	for _, key := range Keys() {
		if same[key] {
			continue
		}
		if catalogue[English][key] == catalogue[Spanish][key] {
			t.Errorf("%q is identical in both languages: %q", key, catalogue[Spanish][key])
		}
	}
}

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Lang
		ok   bool
	}{
		{"es", Spanish, true},
		{"en", English, true},
		{"", Default, false},
		{"fr", Default, false},
		{"EN", Default, false},
	} {
		got, ok := Parse(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Parse(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAnUnknownKeyShowsItself(t *testing.T) {
	if got := For(English).T("no.such.key"); got != "no.such.key" {
		t.Errorf("an unknown key gave %q", got)
	}
}

// A language missing a message falls back to the base rather than to nothing.
func TestFallingBackToTheBase(t *testing.T) {
	catalogue[English]["test.only.in.base"] = ""
	delete(catalogue[English], "test.only.in.base")
	catalogue[Spanish]["test.only.in.base"] = "sólo en español"
	defer delete(catalogue[Spanish], "test.only.in.base")

	if got := For(English).T("test.only.in.base"); got != "sólo en español" {
		t.Errorf("the fallback gave %q", got)
	}
}
