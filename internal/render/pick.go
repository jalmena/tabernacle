package render

import (
	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/harmony"
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/song"
)

// PickView is what the editor's chord chooser offers for one chord: that chord
// on each of the twelve notes, each of the usual kinds of chord on its own note,
// and each note it could stand over.
//
// Every choice carries the whole symbol it would make, written the way a song
// file writes it. The editor only ever copies one of them into place, so that
// what a chord becomes is decided here and nowhere else.
type PickView struct {
	// Symbol is the chord as it will be written, and empty when what was typed
	// is not one. Then it is an annotation, a repeat or a "let ring", and there
	// is nothing to choose between.
	Symbol    string
	Roots     []Choice
	Qualities []Choice
	Basses    []Choice
}

// Pick reads what somebody typed where a chord goes and lays out what it could
// be changed into.
//
// The notes are letters whatever the reader prefers to see on a sheet, because
// the editor shows what the file holds.
func Pick(lang i18n.Lang, typed string) PickView {
	say := i18n.For(lang)
	c, ok := song.TypedChord(typed)
	if !ok {
		// Nothing to keep, so each note offers the plain chord on it.
		v := PickView{Roots: make([]Choice, 0, 12)}
		for pc := range 12 {
			name := chord.Spell(pc, chord.SpellingFor(pc, false)).Format(chord.English)
			v.Roots = append(v.Roots, Choice{Label: name, Value: name})
		}
		return v
	}

	v := PickView{Symbol: c.String(), Roots: make([]Choice, 0, 12)}

	// The same chord somewhere else is the chord moved there whole, bass and
	// all: C/E on D is D/F#, and anything else would leave a bass that belongs
	// to another chord.
	at := c.Root.PitchClass()
	for pc := range 12 {
		moved := c
		if pc != at {
			moved = c.Transpose(pc-at, chord.SpellingFor(pc, c.IsMinor()))
		}
		v.Roots = append(v.Roots, Choice{
			Label: moved.Root.Format(chord.English),
			Value: moved.String(),
			On:    pc == at,
		})
	}

	root := c.Root.Format(chord.English)
	bass := ""
	if c.HasBass {
		bass = "/" + c.Bass.Format(chord.English)
	}
	// Which of the kinds on offer this chord already is, however it is spelled:
	// Amin and A- are both the minor one.
	family, red, known := harmony.Of(c)
	known = known && red.Exact()
	found := false
	for _, q := range qualityChoices {
		kind, _, _ := harmony.Of(chord.Chord{Root: c.Root, Suffix: q.suffix})
		on := known && kind == family
		found = found || on
		v.Qualities = append(v.Qualities, Choice{
			Label: root + q.suffix + " · " + say.T(q.key),
			Value: root + q.suffix + bass,
			On:    on,
		})
	}
	if !found {
		// A kind this list has no name for is still the one that is there, and
		// the list has to be able to say so.
		v.Qualities = append([]Choice{{Label: root + c.Suffix, Value: c.String(), On: true}}, v.Qualities...)
	}

	plain := c
	plain.Bass, plain.HasBass = chord.Note{}, false
	v.Basses = append(v.Basses, Choice{
		Label: say.T("editor.chord.rootbass"),
		Value: plain.String(),
		On:    !c.HasBass,
	})
	spelling := chord.SpellingForKey(c)
	for step := 1; step < 12; step++ {
		note := chord.Spell(at+step, spelling)
		on := c.HasBass && c.Bass.PitchClass() == note.PitchClass()
		if on {
			note = c.Bass // as it is written, Gb rather than F#
		}
		over := plain
		over.Bass, over.HasBass = note, true
		v.Basses = append(v.Basses, Choice{Label: note.Format(chord.English), Value: over.String(), On: on})
	}
	return v
}
