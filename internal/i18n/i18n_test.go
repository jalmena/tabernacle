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

// A language file that is word for word the Spanish is a copy somebody made and
// never translated.
//
// Judged as a share and not key by key, because between Spanish and Catalan or
// Galician a great many single words genuinely coincide — "Administrador" is
// "Administrador" — and a list of those exceptions would be longer than the
// thing it guards. What no real translation does is agree on half the file.
func TestNoLanguageIsACopyOfTheBase(t *testing.T) {
	const tooMuch = 0.40
	for _, l := range All() {
		if l == Spanish {
			continue
		}
		same := 0
		for _, key := range Keys() {
			if catalogue[l][key] == spanish[key] {
				same++
			}
		}
		if share := float64(same) / float64(len(Keys())); share > tooMuch {
			t.Errorf("%s repeats the Spanish word for word in %.0f%% of its keys, which is a copy rather than a translation",
				l, share*100)
		}
	}
}

// A language that is not one of ours is not one of ours, whatever it looks like.
func TestValid(t *testing.T) {
	for in, want := range map[Lang]bool{"es": true, "en": true, "ca": true, "": false, "xx": false, "EN": false} {
		if got := in.Valid(); got != want {
			t.Errorf("Lang(%q).Valid() = %v, want %v", in, got, want)
		}
	}
	// An unknown one still renders, in the base language, rather than blank.
	if got := For("xx").T("nav.songs"); got != "Canciones" {
		t.Errorf("an unknown language gave %q", got)
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
