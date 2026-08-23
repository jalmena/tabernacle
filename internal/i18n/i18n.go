// Package i18n holds the interface's words in more than one language.
//
// Messages are looked up by key rather than by their Spanish text, so adding a
// language is adding a map and nothing else. Spanish is the base: a key missing
// from another language falls back to it, so a half-finished translation shows a
// Spanish sentence rather than a blank or a key. A test asserts none is missing,
// which is what keeps that fallback a safety net rather than a habit.
//
// The translations are written, not transliterated. "Añadir" is "Add", not "To
// add"; "Suena" would have been "Sounds", which is not what a guitarist says.
// Where the two languages disagree about how to put something, the English says
// what an English-speaking musician would say.
package i18n

import (
	"fmt"
	"slices"
)

// Lang is a language the interface is available in.
type Lang string

const (
	// Spanish is the base. Every key exists here.
	Spanish    Lang = "es"
	English    Lang = "en"
	Catalan    Lang = "ca"
	Galician   Lang = "gl"
	Basque     Lang = "eu"
	Portuguese Lang = "pt"
	French     Lang = "fr"
	Italian    Lang = "it"
	German     Lang = "de"
	Dutch      Lang = "nl"
	Polish     Lang = "pl"
	Russian    Lang = "ru"
	Turkish    Lang = "tr"
	Indonesian Lang = "id"
	Chinese    Lang = "zh"
	Japanese   Lang = "ja"
	Korean     Lang = "ko"
	Hindi      Lang = "hi"
	Arabic     Lang = "ar"
)

// Default is the language of an interface nobody has chosen one for.
const Default = Spanish

// names are what a language calls itself, which is what belongs in a chooser: a
// reader looking for their own language recognises it written their way.
var names = map[Lang]string{
	Spanish:    "Español",
	English:    "English",
	Catalan:    "Català",
	Galician:   "Galego",
	Basque:     "Euskara",
	Portuguese: "Português",
	French:     "Français",
	Italian:    "Italiano",
	German:     "Deutsch",
	Dutch:      "Nederlands",
	Polish:     "Polski",
	Russian:    "Русский",
	Turkish:    "Türkçe",
	Indonesian: "Bahasa Indonesia",
	Chinese:    "中文",
	Japanese:   "日本語",
	Korean:     "한국어",
	Hindi:      "हिन्दी",
	Arabic:     "العربية",
}

// rtl is the languages written right to left. There is one for now; the set
// exists so that adding a second is data rather than a condition.
var rtl = map[Lang]bool{Arabic: true}

// All lists the languages, base first and then by how close they are to it: the
// languages of Spain, then the rest of Europe, then further out. A chooser reads
// down this list, and alphabetical order by code would put Arabic above Spanish
// for no reason anybody could see.
func All() []Lang {
	return []Lang{Spanish, English, Catalan, Galician, Basque}
}

// Dir is the direction this language is written in, as an HTML dir attribute.
//
// It belongs to the language and not to the page, which is why it lives here:
// the header, the fretboard and the chord list all have to agree, and they only
// can if there is one answer.
func (l Lang) Dir() string {
	if rtl[l] {
		return "rtl"
	}
	return "ltr"
}

// Codes lists the languages as their codes, for a message that has to name them
// all.
func Codes() []string {
	out := make([]string, 0, len(All()))
	for _, l := range All() {
		out = append(out, string(l))
	}
	return out
}

// Name is what this language calls itself.
func (l Lang) Name() string {
	if n, ok := names[l]; ok {
		return n
	}
	return string(l)
}

// Valid reports whether this is a language we have. The empty string is not: it
// means "unset", and the caller decides what that becomes.
func (l Lang) Valid() bool { return slices.Contains(All(), l) }

// Printer renders messages in one language.
type Printer struct{ lang Lang }

// For returns a printer, falling back to the base language for anything unknown.
func For(l Lang) *Printer {
	if !l.Valid() {
		l = Default
	}
	return &Printer{lang: l}
}

// Lang is the language this printer renders in.
func (p *Printer) Lang() Lang { return p.lang }

// T renders a message. Extra arguments are formatted into it.
//
// An unknown key returns itself. That is deliberately ugly: a key on the page is
// a mistake somebody should see and fix, and it is better than a blank space
// nobody notices.
func (p *Printer) T(key string, args ...any) string {
	s, ok := catalogue[p.lang][key]
	if !ok {
		if s, ok = catalogue[Spanish][key]; !ok {
			return key
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// Has reports whether a key exists in the base language. Tests use it; nothing
// else should need to ask.
func Has(key string) bool {
	_, ok := catalogue[Spanish][key]
	return ok
}

// Keys lists every key in the base language, sorted.
func Keys() []string {
	out := make([]string, 0, len(catalogue[Spanish]))
	for k := range catalogue[Spanish] {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
