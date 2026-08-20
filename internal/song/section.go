package song

import "strings"

// Kind is the musical role of a section.
//
// It drives styling, navigation and print layout. The label the user wrote is
// kept separately and verbatim, so an unrecognised heading is never lost or
// rewritten.
type Kind string

const (
	KindIntro     Kind = "intro"
	KindVerse     Kind = "verse"
	KindPreChorus Kind = "pre-chorus"
	KindChorus    Kind = "chorus"
	KindBridge    Kind = "bridge"
	KindSolo      Kind = "solo"
	KindInterlude Kind = "interlude"
	KindOutro     Kind = "outro"
	KindTab       Kind = "tab"
	KindOther     Kind = "other"
)

// kindAliases maps a normalised label onto a kind. Spanish labels are here
// because a good half of the sheets this thing will hold are written that way,
// and a Spanish heading should still get chorus styling.
var kindAliases = map[string]Kind{
	"intro":         KindIntro,
	"introduction":  KindIntro,
	"introduccion":  KindIntro,
	"introducción":  KindIntro,
	"verse":         KindVerse,
	"estrofa":       KindVerse,
	"verso":         KindVerse,
	"prechorus":     KindPreChorus,
	"preestribillo": KindPreChorus,
	"chorus":        KindChorus,
	"refrain":       KindChorus,
	"estribillo":    KindChorus,
	"coro":          KindChorus,
	"bridge":        KindBridge,
	"puente":        KindBridge,
	"solo":          KindSolo,
	"instrumental":  KindSolo,
	"lead":          KindSolo,
	"interlude":     KindInterlude,
	"interludio":    KindInterlude,
	"outro":         KindOutro,
	"ending":        KindOutro,
	"coda":          KindOutro,
	"final":         KindOutro,
	"cierre":        KindOutro,
	"tab":           KindTab,
	"tablature":     KindTab,
	"tablatura":     KindTab,
	"riff":          KindTab,
}

// KindForLabel guesses the kind of a section from its heading.
//
// "Verse 2", "verse", "VERSE II", "Estrofa 2" and "Chorus (x2)" all resolve to
// their kind; anything unrecognised becomes KindOther and keeps its label.
func KindForLabel(label string) Kind {
	if k, ok := kindAliases[normalizeLabel(label)]; ok {
		return k
	}
	return KindOther
}

// normalizeLabel reduces a heading to a lookup key: lowercase, no punctuation,
// no parenthesised aside, no trailing numbering, no spaces.
func normalizeLabel(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_', r == '\t':
			return ' '
		case r > 127: // keep accented letters such as the one in "introducción"
			return r
		}
		return -1
	}, s)
	fields := strings.Fields(s)
	for len(fields) > 1 && isNumbering(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, "")
}

// romanNumerals are the ones that plausibly appear on a chord sheet.
var romanNumerals = map[string]bool{
	"i": true, "ii": true, "iii": true, "iv": true, "v": true,
	"vi": true, "vii": true, "viii": true, "ix": true, "x": true,
}

// isNumbering reports whether a trailing word is a section number rather than
// part of the name: "2", "ii", "x2", "b".
func isNumbering(w string) bool {
	if w == "" {
		return false
	}
	if romanNumerals[w] {
		return true
	}
	if len(w) == 1 && w[0] >= 'a' && w[0] <= 'z' {
		return true
	}
	if w[0] == 'x' {
		w = w[1:]
	}
	if w == "" {
		return false
	}
	for i := 0; i < len(w); i++ {
		if w[i] < '0' || w[i] > '9' {
			return false
		}
	}
	return true
}
