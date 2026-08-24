package render

import (
	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// howManyCapos is how many places to offer. One is too few to choose from and
// four is a table; a player tries the first, and the others are there because
// the first may not suit their voice.
const howManyCapos = 3

// capoAdvice turns the ranking into something a page can show, and decides
// whether it is worth showing at all.
//
// The rule for staying quiet matters more than the advice. Most songs are
// already open chords, and a panel that appears on all of them saying "you
// could also play this in a different position" is noise that teaches the
// reader to stop looking at that corner of the page. So an option is offered
// only when it removes work — a barre, or a chord we hold no shape for — and
// never merely because it scores a point lower.
func capoAdvice(say *i18n.Printer, key chord.Chord, chords []chord.Chord, at int, notation chord.Notation, vo voicing.Options) []CapoView {
	ranked := voicing.CapoOptions(key, chords, at, vo)
	if len(ranked) == 0 {
		return nil
	}

	var now voicing.CapoOption
	for _, o := range ranked {
		if o.Fret == at {
			now = o
			break
		}
	}

	var out []CapoView
	for _, o := range ranked {
		if len(out) == howManyCapos {
			break
		}
		if o.Fret == at {
			continue
		}
		if o.Barred >= now.Barred && o.Unknown >= now.Unknown {
			continue
		}
		out = append(out, capoView(say, o, notation))
	}
	return out
}

func capoView(say *i18n.Printer, o voicing.CapoOption, notation chord.Notation) CapoView {
	v := CapoView{Fret: o.Fret, InKey: say.T("capo.inkey", o.Key.Format(notation))}
	if o.Fret == 0 {
		v.Where = say.T("capo.none")
	} else {
		v.Where = say.T("capo.fret", o.Fret)
	}

	for _, c := range o.Chords {
		v.Chords = append(v.Chords, CapoChord{Text: c.Format(notation), Query: c.String()})
	}

	switch {
	case o.Unknown > 0:
		v.Cost = say.T("capo.unknown", o.Unknown)
	case o.Barred == 0 && o.Open == len(o.Chords):
		v.Cost = say.T("capo.allopen")
	case o.Barred == 0:
		v.Cost = say.T("capo.nobarre")
	case o.Barred == 1:
		v.Cost = say.T("capo.barre.one")
	default:
		v.Cost = say.T("capo.barre.many", o.Barred)
	}
	return v
}

// keyOf is the key a sheet should be spelled by: what it declares, or its first
// chord when it declares nothing.
func keyOf(s song.Song) (chord.Chord, bool) {
	if s.Key != "" {
		if c, err := chord.Parse(s.Key); err == nil {
			return c, true
		}
	}
	if all := s.Chords(); len(all) > 0 {
		return all[0], true
	}
	return chord.Chord{}, false
}
