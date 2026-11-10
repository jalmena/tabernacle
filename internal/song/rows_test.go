package song

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestARowIsTheWordsAndWhereEachBracketStood(t *testing.T) {
	cases := []struct {
		name, body string
		want       []Row
	}{
		{"chords inside the words", "A[G]mazing grace how [C]sweet",
			[]Row{{Words: "Amazing grace how sweet", Marks: []Mark{{At: 1, Text: "G", Chord: true}, {At: 18, Text: "C", Chord: true}}}}},
		{"an accent in front of a chord", "Can[Am]ción [x2]",
			[]Row{{Words: "Canción ", Marks: []Mark{{At: 3, Text: "Am", Chord: true}, {At: 8, Text: "x2"}}}}},
		// A guitar is two code units to a browser and four bytes here.
		{"an emoji in front of a chord", "🎸[C]la",
			[]Row{{Words: "🎸la", Marks: []Mark{{At: 2, Text: "C", Chord: true}}}}},
		{"two brackets in one place", "la [C][x2]la",
			[]Row{{Words: "la la", Marks: []Mark{{At: 3, Text: "C", Chord: true}, {At: 3, Text: "x2"}}}}},
		{"chords with no words under them", "[Am] [F]  [C]",
			[]Row{{Words: "   ", Marks: []Mark{{At: 0, Text: "Am", Chord: true}, {At: 1, Text: "F", Chord: true}, {At: 3, Text: "C", Chord: true}}}}},
		{"a chord spelled loosely", "[ Am ]la",
			[]Row{{Words: "la", Marks: []Mark{{At: 0, Text: "Am", Chord: true}}}}},
		{"escapes are resolved", `\[sic\] and a \\ and a \# too`,
			[]Row{{Words: `[sic] and a \ and a # too`}}},
		{"a blank line between stanzas", "la\n\nla",
			[]Row{{Words: "la"}, {}, {Words: "la"}}},
		{"a fenced block is one row", "la\n```tab\ne|-0-|\nB|-1-|\n```\nla",
			[]Row{{Words: "la"}, {Words: "e|-0-|\nB|-1-|", Fenced: true, Info: "tab"}, {Words: "la"}}},
		{"a heading inside a fence is tablature", "````\n## not a heading\n```\n````",
			[]Row{{Words: "## not a heading\n```", Fenced: true}}},
		{"a section with nothing in it", "", []Row{{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Rows(c.body)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("laid out as\n%#v\nwant\n%#v", got, c.want)
			}
		})
	}
}

// What cannot be laid out is a mistake in the text, and is left as text.
func TestABodyWithAMistakeInItIsNotLaidOut(t *testing.T) {
	for name, body := range map[string]string{
		"a bracket left open":     "la [C la",
		"a fence left open":       "```tab\ne|-0-|",
		"a heading":               "la\n## Chorus\nla",
		"a reserved line":         "# later",
		"bytes that are not text": "la \xff[C]la",
	} {
		if rows, err := Rows(body); err == nil {
			t.Errorf("%s was laid out as %#v", name, rows)
		}
	}
}

// The promise the editor stands on: rows that nobody moved go back to the bytes
// they came from.
func TestRowsGoBackToTheSameBytes(t *testing.T) {
	files, _ := filepath.Glob("../../examples/songs/*/*.song")
	files = append(files, "testdata/canonical.song")
	sections := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := Take(raw)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, sec := range p.Sections {
			sections++
			rows, err := Rows(sec.Body)
			if err != nil {
				t.Errorf("%s, %q: %v", f, sec.Label, err)
				continue
			}
			if got := FromRows(rows); got != sec.Body {
				t.Errorf("%s, %q came back as\n%s\nwant\n%s", f, sec.Label, got, sec.Body)
			}
		}
	}
	if sections < 8 {
		t.Fatalf("only %d sections were tried; this proves little", sections)
	}
}

func TestRowsAreWrittenTheWayTheFormatWantsThem(t *testing.T) {
	cases := []struct {
		name string
		rows []Row
		want string
	}{
		{"a chord moved along the words",
			[]Row{{Words: "Amazing grace", Marks: []Mark{{At: 8, Text: "G"}}}}, "Amazing [G]grace"},
		{"marks in any order",
			[]Row{{Words: "la la", Marks: []Mark{{At: 3, Text: "C"}, {At: 0, Text: "G"}}}}, "[G]la [C]la"},
		{"marks in one place keep their order",
			[]Row{{Words: "la", Marks: []Mark{{At: 2, Text: "C"}, {At: 2, Text: "x2"}}}}, "la[C][x2]"},
		{"past the end is the end",
			[]Row{{Words: "la", Marks: []Mark{{At: 99, Text: "C"}}}}, "la[C]"},
		{"before the start is the start",
			[]Row{{Words: "la", Marks: []Mark{{At: -3, Text: "C"}}}}, "[C]la"},
		{"counted the way a browser counts",
			[]Row{{Words: "🎸la", Marks: []Mark{{At: 2, Text: "C"}}}}, "🎸[C]la"},
		{"never inside a character",
			[]Row{{Words: "🎸la", Marks: []Mark{{At: 1, Text: "C"}}}}, "🎸[C]la"},
		{"a bracket with nothing in it is not written",
			[]Row{{Words: "la", Marks: []Mark{{At: 0, Text: ""}, {At: 1, Text: " "}}}}, "la"},
		{"a mark cannot close itself early",
			[]Row{{Words: "la", Marks: []Mark{{At: 0, Text: "x]2\n"}}}}, "[x2]la"},
		{"what is not a chord is an annotation",
			[]Row{{Words: "la", Marks: []Mark{{At: 2, Text: "let ring"}}}}, "la[let ring]"},
		{"a chord is written the one way",
			[]Row{{Words: "la", Marks: []Mark{{At: 0, Text: " A♭m "}}}}, "[Abm]la"},
		{"a name nobody has read yet is read as a typed one",
			[]Row{{Words: "la la", Marks: []Mark{{At: 0, Text: "lam", Typed: true}, {At: 3, Text: "sol7/si", Typed: true}}}}, "[Am]la [G7/B]la"},
		{"and is an annotation when it is no chord however it is read",
			[]Row{{Words: "la", Marks: []Mark{{At: 2, Text: "bis", Typed: true}}}}, "la[bis]"},
		{"a name that was not typed is left as it is written",
			[]Row{{Words: "la", Marks: []Mark{{At: 0, Text: "lam"}, {At: 2, Text: "am"}}}}, "[lam]la[am]"},
		{"brackets and backslashes in the words are escaped",
			[]Row{{Words: `a [b] c\d`}}, `a \[b] c\\d`},
		{"words that would read as a heading are escaped",
			[]Row{{Words: "# one"}, {Words: "``` two"}}, "\\# one\n\\``` two"},
		{"a row is one line",
			[]Row{{Words: "one\ntwo\r\nthree"}}, "one two three"},
		{"chords alone on a line",
			[]Row{{Words: "  ", Marks: []Mark{{At: 0, Text: "Am"}, {At: 1, Text: "F"}, {At: 2, Text: "C"}}}}, "[Am] [F] [C]"},
		{"a fenced block",
			[]Row{{Words: "e|-0-|\nB|-1-|", Fenced: true, Info: "tab"}}, "```tab\ne|-0-|\nB|-1-|\n```"},
		{"a fence longer than what it holds",
			[]Row{{Words: "```\nx", Fenced: true, Info: "t`ab\n"}}, "````tab\n```\nx\n````"},
		{"an empty fenced block",
			[]Row{{Fenced: true}}, "```\n```"},
		{"blank rows between stanzas",
			[]Row{{Words: "la"}, {}, {Words: "la"}}, "la\n\nla"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromRows(c.rows)
			if got != c.want {
				t.Fatalf("written as %q, want %q", got, c.want)
			}
			// Whatever was sent, what is written reads back as a body.
			if _, err := Rows(got); err != nil {
				t.Errorf("%q does not read back: %v", got, err)
			}
		})
	}
}

// The rows travel to the browser and back as JSON, so that has to be a round
// trip too.
func TestRowsSurviveTheTripToTheBrowser(t *testing.T) {
	body := "A[G]mazing 🎸 gr[x2]ace\n\n```tab\ne|-0-|\n```\n\\[sic]"
	rows, err := Rows(body)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var back []Row
	if err := json.Unmarshal(wire, &back); err != nil {
		t.Fatal(err)
	}
	if got := FromRows(back); got != body {
		t.Errorf("came back as %q\nover the wire: %s", got, wire)
	}
}

// FuzzRows checks that laying a body out and writing it back never changes what
// the body says, and that what is written is already in its final form.
func FuzzRows(f *testing.F) {
	f.Add("A[G]mazing grace [x2]\n\nmore")
	f.Add("```tab\ne|-0-|\n```\nla")
	f.Add("\\[sic\\] \\\\ \\# 🎸[C]la")
	f.Add("[Am] [F]\r\n\\ ")
	f.Add("````\n```\n````")
	f.Add("la [C la")
	f.Add("")

	const front = "---\ntitle: T\n---\n"
	f.Fuzz(func(t *testing.T, body string) {
		rows, err := Rows(body)
		if err != nil {
			return
		}
		out := FromRows(rows)

		again, err := Rows(out)
		if err != nil {
			t.Fatalf("what was written does not read back: %v\nbody: %q\nwritten: %q", err, body, out)
		}
		if twice := FromRows(again); twice != out {
			t.Fatalf("writing is not settled\nbody: %q\nfirst: %q\nsecond: %q", body, out, twice)
		}

		// An empty bracket is the one thing deliberately not written back.
		for _, row := range rows {
			for _, m := range row.Marks {
				if strings.TrimSpace(m.Text) == "" {
					return
				}
			}
		}
		before, err := Draft([]byte(front + body))
		if err != nil {
			t.Fatalf("laid out a body the reader refuses: %v\nbody: %q", err, body)
		}
		after, err := Draft([]byte(front + out))
		if err != nil {
			t.Fatalf("wrote a body the reader refuses: %v\nbody: %q\nwritten: %q", err, body, out)
		}
		if !reflect.DeepEqual(before.Sections, after.Sections) {
			t.Fatalf("the section changed\nbody: %q\nwritten: %q", body, out)
		}

		wire, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		var back []Row
		if err := json.Unmarshal(wire, &back); err != nil {
			t.Fatal(err)
		}
		if got := FromRows(back); got != out {
			t.Fatalf("the trip to the browser changed it\nbody: %q\nwritten: %q\nafter: %q", body, out, got)
		}
	})
}
