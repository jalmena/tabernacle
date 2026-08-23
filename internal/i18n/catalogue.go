package i18n

// catalogue is every message, by language.
//
// One map per language, each in its own file, so that adding a language really
// is adding a map: nothing else in the package changes, and a translator can be
// handed one file. Spanish is the base and the only one guaranteed complete; a
// key missing anywhere else falls back to it, so a half-finished translation
// shows a Spanish sentence rather than a blank or a key. A test asserts none is
// missing, which is what keeps that fallback a safety net rather than a habit.
var catalogue = map[Lang]map[string]string{
	Spanish:    spanish,
	English:    english,
	Catalan:    catalan,
	Galician:   galician,
	Basque:     basque,
	Portuguese: portuguese,
	French:     french,
	Italian:    italian,
	German:     german,
}
