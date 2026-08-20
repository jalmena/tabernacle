package render

import (
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/song"
)

func parse(t *testing.T, src string) song.Song {
	t.Helper()
	s, err := song.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

const sample = `---
format: 1
title: Amazing Grace
artist: Traditional
key: G
capo: 2
tempo: 90
meter: 3/4
tags: [hymn, example]
notes: |-
  First note line.
  Second note line.
---

## Verse 1
A[G]mazing grace how [C]sweet the [G]sound
That saved a [Em]wretch like [D]me [x2]

I [G]once was lost but [C]now am [G]found

## Solo
` + "```tab" + `
e|---0---3---|
B|---1---0---|
` + "```" + `

## Outro
no chords at all here
`

// reconstruct rebuilds the song's words from the view model, the same way
// song.PlainText builds them: annotations are not words, and a line that
// carries no words at all contributes nothing.
//
// Comparing the two is the strongest available check on the word-and-cell
// transform, because it fails if a single character is dropped, duplicated or
// moved.
func reconstruct(v SheetView) string {
	var out strings.Builder
	for _, sec := range v.Sections {
		for _, blk := range sec.Blocks {
			for _, ln := range blk.Lines {
				var line strings.Builder
				for _, w := range ln.Words {
					for _, c := range w.Cells {
						line.WriteString(c.Text)
					}
					line.WriteString(w.Sep)
				}
				if line.Len() == 0 {
					continue
				}
				out.WriteString(line.String())
				out.WriteByte('\n')
			}
		}
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// The strongest property of the transform: rendering must not lose or invent a
// single character of the words.
func TestLineReconstructionIsExact(t *testing.T) {
	sources := []string{
		"A[G]mazing grace how [C]sweet the [G]sound",
		"[G]Leading chord",
		"Trailing chord [D]",
		"   indented line with [C]chord",
		"double  spaces   between words",
		"[G][C][D] three chords together",
		"word[G]split[C]several[D]times",
		"[x2] annotation first",
		"unicode: corazón está partido [Am]así",
		"trailing spaces   ",
		"[G]",
		"plain words only",
	}
	for _, body := range sources {
		t.Run(body, func(t *testing.T) {
			s := parse(t, "---\nformat: 1\ntitle: T\n---\n\n## Verse 1\n"+body+"\n")
			got := reconstruct(Sheet(s, Options{}))
			want := strings.TrimSuffix(s.PlainText(), "\n")
			if got != want {
				t.Errorf("reconstruction mismatch\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

func TestSheetMetadata(t *testing.T) {
	v := Sheet(parse(t, sample), Options{})
	if v.Title != "Amazing Grace" || v.Artist != "Traditional" {
		t.Errorf("title/artist = %q/%q", v.Title, v.Artist)
	}
	if v.Key != "G" || v.OrigKey != "G" {
		t.Errorf("key = %q, original = %q", v.Key, v.OrigKey)
	}
	if v.Capo != 2 || v.Tempo != 90 || v.Meter != "3/4" {
		t.Errorf("capo/tempo/meter = %d/%d/%q", v.Capo, v.Tempo, v.Meter)
	}
	if len(v.Notes) != 2 || v.Notes[0] != "First note line." {
		t.Errorf("notes = %v, want one entry per line", v.Notes)
	}
	if v.Shifted {
		t.Error("Shifted is true for an untransposed sheet")
	}
	if v.Notation != "english" {
		t.Errorf("notation = %q", v.Notation)
	}
}

func TestChordsUsedIsDerivedAndOrdered(t *testing.T) {
	v := Sheet(parse(t, sample), Options{})
	var got []string
	for _, c := range v.Chords {
		got = append(got, c.Text)
	}
	if strings.Join(got, " ") != "G C Em D" {
		t.Errorf("chords used = %v, want first-appearance order with no duplicates", got)
	}
	// Every entry must carry the same handle its occurrences in the body do, or
	// clicking one would open a different shape.
	ids := map[string]bool{}
	for _, c := range v.Chords {
		ids[c.ID] = true
		if c.ID == "" {
			t.Errorf("chord %q has no id", c.Text)
		}
	}
	for _, sec := range v.Sections {
		for _, blk := range sec.Blocks {
			for _, ln := range blk.Lines {
				for _, w := range ln.Words {
					for _, cell := range w.Cells {
						if cell.Chord != "" && !ids[cell.ChordID] {
							t.Errorf("chord %q in the body has id %q, which is not in the chords-used panel",
								cell.Chord, cell.ChordID)
						}
					}
				}
			}
		}
	}
}

// A chord landing inside a word must stay inside that word, or the browser will
// break the word in half when the line wraps.
func TestMidWordChordStaysInOneWord(t *testing.T) {
	s := parse(t, "---\nformat: 1\ntitle: T\n---\n\n## Verse 1\nA[G]mazing grace\n")
	v := Sheet(s, Options{})
	line := v.Sections[0].Blocks[0].Lines[0]

	if len(line.Words) < 2 {
		t.Fatalf("got %d words, want at least two", len(line.Words))
	}
	first := line.Words[0]
	if len(first.Cells) != 2 {
		t.Fatalf("the first word has %d cells, want two: 'A' and the chorded 'mazing'", len(first.Cells))
	}
	if first.Cells[0].Text != "A" || first.Cells[0].Chord != "" {
		t.Errorf("first cell = %+v, want the bare letter A", first.Cells[0])
	}
	if first.Cells[1].Chord != "G" || first.Cells[1].Text != "mazing" {
		t.Errorf("second cell = %+v, want G above 'mazing'", first.Cells[1])
	}
	if first.Sep != " " {
		t.Errorf("separator = %q, want a single space", first.Sep)
	}
}

func TestHasChordLetsTheChordRowBeSkipped(t *testing.T) {
	v := Sheet(parse(t, sample), Options{})
	var outro *SectionView
	for i := range v.Sections {
		if v.Sections[i].Label == "Outro" {
			outro = &v.Sections[i]
		}
	}
	if outro == nil {
		t.Fatal("the outro section is missing")
	}
	if outro.Blocks[0].Lines[0].HasChord {
		t.Error("a line with no chords reports HasChord")
	}
	if !v.Sections[0].Blocks[0].Lines[0].HasChord {
		t.Error("a line with chords does not report HasChord")
	}
}

// Blank lines group the words into stanzas, and a stanza is the unit that must
// not be split across a printed page.
func TestBlankLinesBecomeSeparateBlocks(t *testing.T) {
	v := Sheet(parse(t, sample), Options{})
	verse := v.Sections[0]
	if verse.Label != "Verse 1" {
		t.Fatalf("first section is %q", verse.Label)
	}
	if len(verse.Blocks) != 2 {
		t.Fatalf("verse has %d blocks, want two stanzas", len(verse.Blocks))
	}
	if len(verse.Blocks[0].Lines) != 2 || len(verse.Blocks[1].Lines) != 1 {
		t.Errorf("stanza sizes = %d and %d, want 2 and 1",
			len(verse.Blocks[0].Lines), len(verse.Blocks[1].Lines))
	}
	// No block may contain a blank line: blank lines are the boundaries.
	for _, b := range verse.Blocks {
		for _, ln := range b.Lines {
			if len(ln.Words) == 0 {
				t.Error("a block contains an empty line")
			}
		}
	}
}

func TestVerbatimBlockIsItsOwnBlock(t *testing.T) {
	v := Sheet(parse(t, sample), Options{})
	var solo *SectionView
	for i := range v.Sections {
		if v.Sections[i].Label == "Solo" {
			solo = &v.Sections[i]
		}
	}
	if solo == nil {
		t.Fatal("the solo section is missing")
	}
	if len(solo.Blocks) != 1 || !solo.Blocks[0].Verbatim {
		t.Fatalf("solo blocks = %+v, want one verbatim block", solo.Blocks)
	}
	b := solo.Blocks[0]
	if b.Info != "tab" {
		t.Errorf("fence tag = %q, want tab", b.Info)
	}
	if !strings.Contains(b.Text, "e|---0---3---|") {
		t.Errorf("verbatim text = %q", b.Text)
	}
	if len(b.Lines) != 0 {
		t.Error("a verbatim block should not also carry parsed lines")
	}
}

func TestAnnotationsAreNeverChords(t *testing.T) {
	s := parse(t, "---\nformat: 1\ntitle: T\n---\n\n## Verse 1\n[G]word [x2] [N.C.] [Chorus]\n")
	v := Sheet(s, Options{Transpose: 2})

	var annotations, chords []string
	for _, w := range v.Sections[0].Blocks[0].Lines[0].Words {
		for _, c := range w.Cells {
			if c.Annotation != "" {
				annotations = append(annotations, c.Annotation)
			}
			if c.Chord != "" {
				chords = append(chords, c.Chord)
			}
		}
	}
	if strings.Join(annotations, "|") != "x2|N.C.|Chorus" {
		t.Errorf("annotations = %v, want them kept verbatim and untransposed", annotations)
	}
	if strings.Join(chords, "|") != "A" {
		t.Errorf("chords = %v, want only the real chord, transposed", chords)
	}
	if len(v.Chords) != 1 {
		t.Errorf("chords used = %v, want annotations left out", v.Chords)
	}
}

func TestTransposition(t *testing.T) {
	v := Sheet(parse(t, sample), Options{Transpose: 2})
	if v.Key != "A" {
		t.Errorf("key = %q, want A", v.Key)
	}
	if v.OrigKey != "G" {
		t.Errorf("original key = %q, want G to still be reported", v.OrigKey)
	}
	if !v.Shifted {
		t.Error("Shifted is false for a transposed sheet")
	}
	var got []string
	for _, c := range v.Chords {
		got = append(got, c.Text)
	}
	if strings.Join(got, " ") != "A D F#m E" {
		t.Errorf("transposed chords = %v, want A D F#m E", got)
	}
	// The capo is a piece of metal, not a chord symbol; transposing must not
	// pretend to move it.
	if v.Capo != 2 {
		t.Errorf("capo = %d, want it untouched at 2", v.Capo)
	}
}

func TestTransposeNormalisation(t *testing.T) {
	tests := []struct{ in, want int }{
		{0, 0}, {2, 2}, {-2, -2}, {11, 11}, {-11, -11},
		{12, 0}, {-12, 0}, {24, 0}, {13, 1}, {-13, -1},
		{14, 2}, {-14, -2}, {23, 11}, {-23, -11},
	}
	for _, tt := range tests {
		if got := NormalizeTranspose(tt.in); got != tt.want {
			t.Errorf("NormalizeTranspose(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
	// A whole octave is the same sheet, so it must not claim to have shifted it.
	if v := Sheet(parse(t, sample), Options{Transpose: 12}); v.Shifted {
		t.Error("transposing by an octave reports the sheet as shifted")
	}
}

func TestSpellingIsConsistentAcrossTheWholeSheet(t *testing.T) {
	// E major up one semitone is F, which is conventionally written with flats.
	src := "---\nformat: 1\ntitle: T\nkey: E\n---\n\n## Verse 1\n[E]one [A]two [B]three [C#m]four\n"
	v := Sheet(parse(t, src), Options{Transpose: 1})
	var got []string
	for _, c := range v.Chords {
		got = append(got, c.Text)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "#") && strings.Contains(joined, "b") {
		t.Errorf("chords = %v; a sheet must not mix sharps and flats", got)
	}
	if joined != "F Bb C Dm" {
		t.Errorf("chords = %v, want F Bb C Dm", got)
	}
}

func TestSolfegeNotation(t *testing.T) {
	v := Sheet(parse(t, sample), Options{Notation: chord.Solfege})
	var got []string
	for _, c := range v.Chords {
		got = append(got, c.Text)
	}
	if strings.Join(got, " ") != "Sol Do Mim Re" {
		t.Errorf("chords = %v, want solfege names", got)
	}
	if v.Key != "Sol" {
		t.Errorf("key = %q, want Sol", v.Key)
	}
	if v.Notation != "solfege" {
		t.Errorf("notation = %q", v.Notation)
	}
	// The handle must not change with the display spelling, or clicking a chord
	// in one notation would open a different shape than in the other.
	english := Sheet(parse(t, sample), Options{})
	for i := range v.Chords {
		if v.Chords[i].ID != english.Chords[i].ID {
			t.Errorf("chord %d has id %q in solfege and %q in english",
				i, v.Chords[i].ID, english.Chords[i].ID)
		}
	}
}

func TestCapoSoundingView(t *testing.T) {
	// Capo 2, written in G. What actually sounds is A.
	written := Sheet(parse(t, sample), Options{})
	sounding := Sheet(parse(t, sample), Options{CapoView: CapoSounding})

	if written.Key != "G" {
		t.Errorf("written key = %q", written.Key)
	}
	if sounding.Key != "A" {
		t.Errorf("sounding key = %q, want A with a capo on the second fret", sounding.Key)
	}
	if !sounding.Shifted {
		t.Error("the sounding view must report that the symbols are not what the file says")
	}
	// The capo field itself is unchanged in both.
	if sounding.Capo != 2 {
		t.Errorf("capo = %d", sounding.Capo)
	}
	// And it composes with transposition.
	both := Sheet(parse(t, sample), Options{CapoView: CapoSounding, Transpose: 1})
	if both.Key != "Bb" {
		t.Errorf("capo 2 plus one semitone = %q, want Bb", both.Key)
	}
}

func TestChordID(t *testing.T) {
	tests := []struct{ sym, want string }{
		{"C", "c"},
		{"Am", "am"},
		{"F#m", "fsm"},
		{"Bb", "bb"},
		{"C#m7", "csm7"},
		{"Cmaj7", "cmaj7"},
		{"C/E", "c_e"},
		{"D/F#", "d_fs"},
		{"C7(b9)", "c7-b9"},
		{"Cm7b5", "cm7b5"},
		{"C+", "c"},
		{"C6/9", "c6_9"},
	}
	for _, tt := range tests {
		c, err := chord.Parse(tt.sym)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.sym, err)
		}
		got := ChordID(c)
		if got != tt.want {
			t.Errorf("ChordID(%q) = %q, want %q", tt.sym, got, tt.want)
		}
		// Whatever it produces has to be safe in an id attribute and a URL.
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				t.Errorf("ChordID(%q) = %q, which contains %q", tt.sym, got, r)
			}
		}
	}
}

func TestSectionAnchorsAreUnique(t *testing.T) {
	src := `---
format: 1
title: T
---

## Chorus
[G]one

## Verse
[C]two

## Chorus
[G]three
`
	v := Sheet(parse(t, src), Options{})
	seen := map[string]bool{}
	for _, sec := range v.Sections {
		if sec.Anchor == "" {
			t.Errorf("section %q has no anchor", sec.Label)
		}
		if seen[sec.Anchor] {
			t.Errorf("anchor %q is used twice", sec.Anchor)
		}
		seen[sec.Anchor] = true
	}
	if v.Sections[0].Anchor != "chorus" || v.Sections[2].Anchor != "chorus-2" {
		t.Errorf("anchors = %q and %q, want chorus and chorus-2",
			v.Sections[0].Anchor, v.Sections[2].Anchor)
	}
	if len(v.Nav) != 3 {
		t.Errorf("nav has %d entries, want one per labelled section", len(v.Nav))
	}
}

func TestUnlabelledSectionIsStillRendered(t *testing.T) {
	v := Sheet(parse(t, "---\nformat: 1\ntitle: T\n---\n[G]straight in with no heading\n"), Options{})
	if len(v.Sections) != 1 {
		t.Fatalf("got %d sections", len(v.Sections))
	}
	if v.Sections[0].Label != "" {
		t.Errorf("label = %q, want empty", v.Sections[0].Label)
	}
	if v.Sections[0].Anchor == "" {
		t.Error("an unlabelled section still needs an anchor")
	}
	// It contributes no jump target, since there is nothing to name.
	if len(v.Nav) != 0 {
		t.Errorf("nav = %v, want empty for an unlabelled section", v.Nav)
	}
}

// Section kinds are used directly as CSS classes, so they must stay in the
// restricted alphabet even for a heading full of punctuation.
func TestSectionKindIsSafeAsAClass(t *testing.T) {
	src := "---\nformat: 1\ntitle: T\n---\n\n## <script>alert(1)</script>\n[G]x\n"
	v := Sheet(parse(t, src), Options{})
	kind := v.Sections[0].Kind
	if kind != "other" {
		t.Errorf("kind = %q, want other for an unrecognised heading", kind)
	}
	// The label keeps the user's text verbatim; escaping it is the template's
	// job, not this package's.
	if v.Sections[0].Label != "<script>alert(1)</script>" {
		t.Errorf("label = %q, want it kept verbatim", v.Sections[0].Label)
	}
	if strings.ContainsAny(v.Sections[0].Anchor, "<>()/") {
		t.Errorf("anchor = %q, which is not safe in an attribute", v.Sections[0].Anchor)
	}
}

func TestEmptySong(t *testing.T) {
	v := Sheet(parse(t, "---\nformat: 1\ntitle: Nothing\n---\n"), Options{})
	if v.Title != "Nothing" {
		t.Errorf("title = %q", v.Title)
	}
	if len(v.Sections) != 0 || len(v.Chords) != 0 || len(v.Nav) != 0 {
		t.Errorf("an empty song produced %d sections, %d chords, %d nav entries",
			len(v.Sections), len(v.Chords), len(v.Nav))
	}
}

// Rendering must never panic and never lose the words, whatever the file holds.
func FuzzSheet(f *testing.F) {
	f.Add(sample)
	f.Add("---\nformat: 1\ntitle: T\n---\n\n## V\n[G]x\n")
	f.Add("---\nformat: 1\ntitle: T\n---\n\n[x2]   [G]\n")
	f.Fuzz(func(t *testing.T, src string) {
		s, err := song.Parse([]byte(src))
		if err != nil {
			return // not a song; the parser's own fuzz target covers that
		}
		for _, o := range []Options{
			{},
			{Transpose: 5},
			{Transpose: -7, Notation: chord.Solfege},
			{CapoView: CapoSounding},
		} {
			got := reconstruct(Sheet(s, o))
			want := strings.TrimSuffix(s.PlainText(), "\n")
			if got != want {
				t.Fatalf("rendering lost or invented text\nopts: %+v\n got: %q\nwant: %q",
					o, got, want)
			}
		}
	})
}
