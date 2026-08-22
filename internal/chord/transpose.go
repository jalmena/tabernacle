package chord

// Spelling selects which accidentals transposition produces.
type Spelling int

const (
	// Sharps spells the black keys as C#, D#, F#, G#, A#.
	Sharps Spelling = iota
	// Flats spells the black keys as Db, Eb, Gb, Ab, Bb.
	Flats
)

var sharpSpelling = [12]Note{
	{'C', 0}, {'C', 1}, {'D', 0}, {'D', 1}, {'E', 0}, {'F', 0},
	{'F', 1}, {'G', 0}, {'G', 1}, {'A', 0}, {'A', 1}, {'B', 0},
}

var flatSpelling = [12]Note{
	{'C', 0}, {'D', -1}, {'D', 0}, {'E', -1}, {'E', 0}, {'F', 0},
	{'G', -1}, {'G', 0}, {'A', -1}, {'A', 0}, {'B', -1}, {'B', 0},
}

// Spell names a pitch class with the given accidentals.
//
// It is the only way to turn a number back into a note, which matters wherever a
// name is derived from sound rather than copied from what someone wrote: a
// chord identified from a fingering has no written spelling to inherit.
func Spell(pitchClass int, sp Spelling) Note {
	if sp == Flats {
		return flatSpelling[wrap(pitchClass)]
	}
	return sharpSpelling[wrap(pitchClass)]
}

func wrap(pitchClass int) int {
	return ((pitchClass % 12) + 12) % 12
}

// Transpose moves the note by the given number of semitones and respells the
// result according to sp.
//
// A shift that lands on the same pitch class returns the note untouched, so a
// no-op transposition never rewrites Gb into F#.
func (n Note) Transpose(semitones int, sp Spelling) Note {
	if semitones%12 == 0 {
		return n
	}
	return Spell(wrap(n.PitchClass()+semitones), sp)
}

// Transpose moves the chord by the given number of semitones, preserving its
// quality suffix and its slash bass.
//
// The caller passes the spelling once for the whole sheet, normally from
// TransposeKey, so that every chord in a song agrees on sharps or flats.
func (c Chord) Transpose(semitones int, sp Spelling) Chord {
	out := c
	out.Root = c.Root.Transpose(semitones, sp)
	if c.HasBass {
		out.Bass = c.Bass.Transpose(semitones, sp)
	}
	return out
}

// majorKeySpelling and minorKeySpelling record the accidentals each key is
// conventionally written with: F major has a flat, D major has sharps, D minor
// has a flat.
//
// They exist so that transposing a sheet into Eb yields Bb rather than A#.
// Both are readable in theory; only one is readable at speed with a guitar in
// your hands.
var majorKeySpelling = [12]Spelling{
	Sharps, // C
	Flats,  // Db
	Sharps, // D
	Flats,  // Eb
	Sharps, // E
	Flats,  // F
	Sharps, // F#
	Sharps, // G
	Flats,  // Ab
	Sharps, // A
	Flats,  // Bb
	Sharps, // B
}

var minorKeySpelling = [12]Spelling{
	Flats,  // Cm
	Sharps, // C#m
	Flats,  // Dm
	Flats,  // Ebm
	Sharps, // Em
	Flats,  // Fm
	Sharps, // F#m
	Flats,  // Gm
	Sharps, // G#m
	Sharps, // Am
	Flats,  // Bbm
	Sharps, // Bm
}

// SpellingFor picks the accidentals that a chord rooted on this pitch class
// conventionally takes: Eb rather than D#, F# rather than Gb.
func SpellingFor(pitchClass int, minor bool) Spelling {
	if minor {
		return minorKeySpelling[pitchClass]
	}
	return majorKeySpelling[pitchClass]
}

// SpellingForKey returns the accidentals a sheet in the given key should use.
//
// An explicit accidental in the key wins: a song the user wrote in Gb is read
// in flats even though F# is the more common spelling of that pitch.
func SpellingForKey(key Chord) Spelling {
	switch {
	case key.Root.Accidental < 0:
		return Flats
	case key.Root.Accidental > 0:
		return Sharps
	}
	return SpellingFor(key.Root.PitchClass(), key.IsMinor())
}

// TransposeKey moves a key symbol and returns the new key together with the
// spelling every chord on the sheet should then use.
//
// Deciding spelling once, from the destination key, is what keeps a transposed
// sheet internally consistent instead of mixing A# and Bb on the same page.
func TransposeKey(key Chord, semitones int) (Chord, Spelling) {
	if semitones%12 == 0 {
		return key, SpellingForKey(key)
	}
	pc := wrap(key.Root.PitchClass() + semitones)
	sp := SpellingFor(pc, key.IsMinor())
	out := key
	out.Root = Spell(pc, sp)
	if key.HasBass {
		out.Bass = key.Bass.Transpose(semitones, sp)
	}
	return out, sp
}
