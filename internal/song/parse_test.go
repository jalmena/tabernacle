package song

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func loadCanonical(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/canonical.song")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return b
}

func TestParseCanonical(t *testing.T) {
	s, err := Parse(loadCanonical(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if s.Format != 1 || s.Title != "Amazing Grace" || s.Artist != "Traditional" {
		t.Errorf("metadata = %+v", s)
	}
	if s.Key != "G" || s.Capo != 2 || s.Tuning != "EADGBE" || s.Tempo != 90 || s.Meter != "3/4" {
		t.Errorf("metadata = %+v", s)
	}
	if got, want := strings.Join(s.Tags, ","), "hymn,public-domain"; got != want {
		t.Errorf("tags = %q, want %q", got, want)
	}
	if !strings.HasPrefix(s.Notes, "Public domain") || !strings.Contains(s.Notes, "\n") {
		t.Errorf("notes = %q", s.Notes)
	}

	if len(s.Sections) != 3 {
		t.Fatalf("got %d sections, want 3", len(s.Sections))
	}
	if s.Sections[0].Kind != KindVerse || s.Sections[0].Label != "Verse 1" {
		t.Errorf("section 0 = %+v", s.Sections[0])
	}
	if s.Sections[1].Kind != KindSolo || s.Sections[2].Kind != KindOutro {
		t.Errorf("kinds = %v, %v", s.Sections[1].Kind, s.Sections[2].Kind)
	}

	// Verse 1: two stanzas of two lines with one blank line between them, and
	// no stray blank line left at either edge.
	verse := s.Sections[0].Lines
	if len(verse) != 5 || verse[2].Kind != LineBlank {
		t.Fatalf("verse lines = %d, %+v", len(verse), verse)
	}
	if verse[0].Kind != LineLyric || verse[4].Kind != LineLyric {
		t.Errorf("verse edges are not lyric lines: %+v", verse)
	}

	// The tab block is one verbatim unit, fence tag included.
	solo := s.Sections[1].Lines
	if len(solo) != 1 || solo[0].Kind != LineVerbatim {
		t.Fatalf("solo lines = %+v", solo)
	}
	if solo[0].Info != "tab" {
		t.Errorf("fence tag = %q, want %q", solo[0].Info, "tab")
	}
	if n := strings.Count(solo[0].Text, "\n"); n != 5 {
		t.Errorf("tab block has %d newlines, want 5", n)
	}
}

func TestParseTokens(t *testing.T) {
	s, err := Parse(loadCanonical(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	toks := s.Sections[0].Lines[0].Tokens
	want := []struct {
		kind TokenKind
		text string
	}{
		{TokenText, "A"},
		{TokenChord, "G"},
		{TokenText, "mazing grace how "},
		{TokenChord, "C"},
		{TokenText, "sweet the "},
		{TokenChord, "G"},
		{TokenText, "sound"},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, w := range want {
		got := toks[i]
		if got.Kind != w.kind {
			t.Errorf("token %d kind = %v, want %v", i, got.Kind, w.kind)
			continue
		}
		text := got.Text
		if got.Kind == TokenChord {
			text = got.Chord.String()
		}
		if text != w.text {
			t.Errorf("token %d = %q, want %q", i, text, w.text)
		}
	}
}

// TestParseAnnotation is the song-level half of the chord package's
// TestParseRejectsText: bracketed text that is not a chord has to stay put.
func TestParseAnnotation(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Verse 1\n[Chorus] [x2] [Am] [] [N.C.]\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var kinds []TokenKind
	var texts []string
	for _, tok := range s.Sections[0].Lines[0].Tokens {
		if tok.Kind == TokenText {
			continue
		}
		kinds = append(kinds, tok.Kind)
		if tok.Kind == TokenChord {
			texts = append(texts, tok.Chord.String())
		} else {
			texts = append(texts, tok.Text)
		}
	}
	wantKinds := []TokenKind{TokenAnnotation, TokenAnnotation, TokenChord, TokenAnnotation, TokenAnnotation}
	wantTexts := []string{"Chorus", "x2", "Am", "", "N.C."}
	for i := range wantKinds {
		if i >= len(kinds) || kinds[i] != wantKinds[i] || texts[i] != wantTexts[i] {
			t.Fatalf("brackets = %v %q, want %v %q", kinds, texts, wantKinds, wantTexts)
		}
	}
}

func TestParseEscapes(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Verse 1\n" +
		`\[not a chord\] and a backslash \\ and \#1 hit` + "\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	toks := s.Sections[0].Lines[0].Tokens
	if len(toks) != 1 || toks[0].Kind != TokenText {
		t.Fatalf("tokens = %+v", toks)
	}
	want := `[not a chord] and a backslash \ and #1 hit`
	if toks[0].Text != want {
		t.Errorf("text = %q, want %q", toks[0].Text, want)
	}
}

func TestParseImplicitSection(t *testing.T) {
	src := "---\ntitle: T\n---\n[C]Just a short thing\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Sections) != 1 || s.Sections[0].Label != "" || s.Sections[0].Kind != KindOther {
		t.Fatalf("sections = %+v", s.Sections)
	}
}

func TestParseFrontMatterForms(t *testing.T) {
	src := strings.Join([]string{
		"---",
		"# a comment line",
		"title: 'Ain''t No Sunshine'",
		`artist: "Bill Withers"`,
		"key: Am",
		"tags:",
		"  - soul",
		"  - cover",
		"notes: |",
		"  first",
		"",
		"  third",
		"---",
		"",
	}, "\n")
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Title != "Ain't No Sunshine" {
		t.Errorf("title = %q", s.Title)
	}
	if s.Artist != "Bill Withers" {
		t.Errorf("artist = %q", s.Artist)
	}
	if strings.Join(s.Tags, ",") != "soul,cover" {
		t.Errorf("tags = %v", s.Tags)
	}
	if s.Notes != "first\n\nthird" {
		t.Errorf("notes = %q", s.Notes)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line int
		want string
	}{
		{"no front matter", "title: T\n", 1, "must start with"},
		{"unterminated front matter", "---\ntitle: T\n", 1, "unterminated front matter"},
		{"unknown field", "---\ntitle: T\ntitel: X\n---\n", 3, `unknown field "titel"`},
		{"duplicate field", "---\ntitle: T\ntitle: X\n---\n", 3, "already set on line 2"},
		{"bad field name", "---\nTitle: T\n---\n", 2, "invalid field name"},
		{"no colon", "---\ntitle\n---\n", 2, "key: value"},
		{"tab indent", "---\ntitle: T\n\tartist: X\n---\n", 3, "spaces, not tabs"},
		{"nested field", "---\ntitle: T\n  artist: X\n---\n", 3, "not nested"},
		{"stray list item", "---\n- soul\n---\n", 2, "does not belong"},
		{"folded scalar", "---\ntitle: T\nnotes: >\n  x\n---\n", 3, "folded block scalars"},
		{"bad number", "---\ntitle: T\ncapo: high\n---\n", 3, "whole number"},
		{"capo range", "---\ntitle: T\ncapo: 13\n---\n", 3, "out of range"},
		{"bad key", "---\ntitle: T\nkey: H\n---\n", 3, "not a chord symbol"},
		{"bad meter", "---\ntitle: T\nmeter: fast\n---\n", 3, "time signature"},
		{"bad tempo", "---\ntitle: T\ntempo: 900\n---\n", 3, "out of range"},
		{"future format", "---\nformat: 2\ntitle: T\n---\n", 2, "unsupported format version 2"},
		{"list for scalar", "---\ntitle: [a, b]\n---\n", 2, "not a list"},
		{"scalar for list", "---\ntitle: T\ntags: soul\n---\n", 3, "takes a list"},
		{"nested list", "---\ntitle: T\ntags: [a, [b]]\n---\n", 3, "nested lists"},
		{"unquoted colon", "---\ntitle: a: b\n---\n", 2, "must be quoted"},
		{"heading no label", "---\ntitle: T\n---\n##\n", 4, `"## Label"`},
		{"heading no space", "---\ntitle: T\n---\n##Verse\n", 4, `"## Label"`},
		{"reserved hash", "---\ntitle: T\n---\n# not a heading\n", 4, "reserved"},
		{"unclosed bracket", "---\ntitle: T\n---\n## V\n[C oops\n", 5, `unclosed "["`},
		{"unterminated fence", "---\ntitle: T\n---\n## V\n```tab\ne|--\n", 5, "unterminated fenced block"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatalf("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("error = %T, want *SyntaxError", err)
			}
			if se.Line != tt.line {
				t.Errorf("error on line %d, want %d (%v)", se.Line, tt.line, err)
			}
		})
	}
}

func TestParseMissingTitle(t *testing.T) {
	if _, err := Parse([]byte("---\nartist: X\n---\n")); err == nil {
		t.Fatal("Parse succeeded without a title, want error")
	}
}

func TestParseCRLF(t *testing.T) {
	src := "\ufeff---\r\ntitle: T\r\n---\r\n\r\n## Verse 1\r\n[C]Hello\r\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Sections) != 1 || len(s.Sections[0].Lines) != 1 {
		t.Fatalf("sections = %+v", s.Sections)
	}
}
