package song

import (
	"reflect"
	"strings"
	"testing"
)

// TestMarshalRoundTrip is the property the application leans on when it saves a
// file the user has been editing by hand: writing back a song that was only
// read must not touch a single byte.
func TestMarshalRoundTrip(t *testing.T) {
	want := loadCanonical(t)
	s, err := Parse(want)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Marshal(Parse(x)) != x\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestReparseIsStable covers the songs the canonical fixture cannot: odd
// values, awkward escapes and nested fences all have to survive a write and a
// read back.
func TestReparseIsStable(t *testing.T) {
	sources := []string{
		// Values a YAML reader would otherwise take for something else.
		"---\ntitle: \"Yes\"\nartist: \"1984\"\n---\n",
		// Text that collides with the body syntax.
		"---\ntitle: T\n---\n\n## Verse 1\n" + `\#1 hit \[bracketed\] back\\slash` + "\n",
		// A fence inside a fence.
		"---\ntitle: T\n---\n\n## Solo\n````tab\n```\n````\n",
		// Annotations, empty brackets and a slash chord on one line.
		"---\ntitle: T\nkey: C\n---\n\n## Verse 1\n[C/E]x [x2] [] [N.C.]\n",
		// Multi-line notes with an internal blank line.
		"---\ntitle: T\nnotes: |-\n  one\n\n  three\n---\n",
		// A tag list and an unlabelled section.
		"---\ntitle: T\ngenre: \"b c\"\n---\n\n[G]No heading here\n",
		// Lyric lines that are only whitespace. Written plainly they would read
		// back as blank lines, and blank lines at the edge of a section are
		// dropped, so the line has to carry an escape.
		"---\ntitle: T\n---\n\n## Verse 1\n\\ \n",
		"---\ntitle: T\n---\n\n## Verse 1\n\\   \n",
		"---\ntitle: T\n---\n\n## Verse 1\n[G]uno\n\\ \n[C]dos\n",
	}

	for _, src := range sources {
		first, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("Parse(%q): %v", src, err)
			continue
		}
		out, err := Marshal(first)
		if err != nil {
			t.Errorf("Marshal(%q): %v", src, err)
			continue
		}
		second, err := Parse(out)
		if err != nil {
			t.Errorf("Parse of marshalled %q: %v\n%s", src, err, out)
			continue
		}
		if !reflect.DeepEqual(first, second) {
			t.Errorf("reparse differs for %q\nfirst:  %+v\nsecond: %+v", src, first, second)
		}
		again, err := Marshal(second)
		if err != nil {
			t.Errorf("second Marshal(%q): %v", src, err)
			continue
		}
		if string(again) != string(out) {
			t.Errorf("marshalling is not idempotent for %q\nfirst:\n%s\nsecond:\n%s", src, out, again)
		}
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	tests := map[string]string{
		"Amazing Grace": "Amazing Grace",
		"F#m":           "F#m",
		"4/4":           "4/4",
		"EADGBE":        "EADGBE",
		"public-domain": "public-domain",
		"Yes":           `"Yes"`,   // a band, not a boolean
		"no":            `"no"`,    // likewise
		"1984":          `"1984"`,  // a title, not a number
		"":              `""`,      //
		" pad ":         `" pad "`, //
		"a: b":          `"a: b"`,
		"hash # here":   `"hash # here"`,
		"-dash":         `"-dash"`,
		"say \"hi\"":    `"say \"hi\""`,
	}
	for in, want := range tests {
		if got := quoteIfNeeded(in); got != want {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarshalRejectsInvalid(t *testing.T) {
	tests := map[string]Song{
		"no title":          {},
		"bad key":           {Title: "T", Key: "H"},
		"capo out of range": {Title: "T", Capo: 99},
		"comma in genre":    {Title: "T", Genre: "a,b"},
		"newline in label":  {Title: "T", Sections: []Section{{Label: "a\nb"}}},
		"bracket in annotation": {Title: "T", Sections: []Section{{Label: "V", Lines: []Line{
			{Kind: LineLyric, Tokens: []Token{{Kind: TokenAnnotation, Text: "a]b"}}},
		}}}},
	}
	for name, s := range tests {
		if _, err := Marshal(s); err == nil {
			t.Errorf("%s: Marshal succeeded, want error", name)
		}
	}
}

func TestMarshalStartsWithTheTitle(t *testing.T) {
	out, err := Marshal(Song{Title: "T"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.HasPrefix(string(out), "---\ntitle: T\n---\n") {
		t.Errorf("output = %q", out)
	}
}
