package song

import (
	"strings"
	"testing"
)

const chordSong = `---
format: 1
title: Test
---

## Intro
[G] [C] [G]

## Verse 1
A[G]mazing grace how [C]sweet the [G]sound
That saved a [Em]wretch like [D]me [x2]

## Solo
` + "```tab" + `
e|---0---3---|
B|---1---0---|
` + "```" + `

## Outro
[Cadd9]Amazing [C]grace, a[G]men
`

func TestSongChords(t *testing.T) {
	s, err := Parse([]byte(chordSong))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var got []string
	for _, c := range s.Chords() {
		got = append(got, c.String())
	}
	// First appearance order, deduplicated, and Cadd9 kept separate from C
	// because it is a different shape.
	want := "G C Em D Cadd9"
	if strings.Join(got, " ") != want {
		t.Errorf("Chords() = %v, want %q", got, want)
	}
}

func TestSongChordsIgnoresAnnotations(t *testing.T) {
	s, err := Parse([]byte("---\nformat: 1\ntitle: T\n---\n\n## Verse 1\n[x2] [N.C.] [Chorus] [let ring] [G]word\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Chords()
	if len(got) != 1 || got[0].String() != "G" {
		t.Errorf("Chords() = %v, want just [G]; annotations are not chords", got)
	}
}

func TestSongChordsEmpty(t *testing.T) {
	s, err := Parse([]byte("---\nformat: 1\ntitle: T\n---\n\n## Verse 1\njust words, no chords\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Chords(); len(got) != 0 {
		t.Errorf("Chords() = %v, want none", got)
	}
}

func TestSongPlainText(t *testing.T) {
	s, err := Parse([]byte(chordSong))
	if err != nil {
		t.Fatal(err)
	}
	got := s.PlainText()

	for _, want := range []string{"Amazing grace how sweet the sound", "wretch like", "Amazing grace, amen"} {
		if !strings.Contains(got, want) {
			t.Errorf("PlainText() is missing %q; got:\n%s", want, got)
		}
	}
	// Chord brackets must be gone, or a search for "G" would match every song.
	for _, unwanted := range []string{"[G]", "[C]", "[Em]", "Cadd9"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("PlainText() still contains %q:\n%s", unwanted, got)
		}
	}
	// Tablature is excluded: nobody searches for fretboard art, and indexing it
	// would dilute the ranking of the words.
	if strings.Contains(got, "e|---0") || strings.Contains(got, "B|---1") {
		t.Errorf("PlainText() included a tab block:\n%s", got)
	}
	// The chord-only intro line contributes no words, so it adds no blank line.
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("PlainText() has a run of blank lines:\n%q", got)
	}
	// The annotation is not words either.
	if strings.Contains(got, "x2") {
		t.Errorf("PlainText() included an annotation:\n%s", got)
	}
}

func TestSongCountLines(t *testing.T) {
	s, err := Parse([]byte(chordSong))
	if err != nil {
		t.Fatal(err)
	}
	// Intro 1, verse 2, outro 1. Blank lines and the tab block do not count.
	if got := s.CountLines(); got != 4 {
		t.Errorf("CountLines() = %d, want 4", got)
	}
}
