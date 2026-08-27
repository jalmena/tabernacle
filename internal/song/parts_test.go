package song

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The promise the field editor stands on: a song taken apart and put back
// together is the same song, and the sections nobody touched are the same text.
func TestASongTakenApartGoesBackTogether(t *testing.T) {
	files, _ := filepath.Glob("../../examples/songs/*/*.song")
	files = append(files, "testdata/canonical.song")
	if len(files) < 2 {
		t.Fatal("no songs to take apart")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			first, err := Parse(raw)
			if err != nil {
				t.Fatalf("not a song: %v", err)
			}
			p, err := Take(raw)
			if err != nil {
				t.Fatalf("cannot be taken apart: %v", err)
			}
			again, err := Parse(Put(p))
			if err != nil {
				t.Fatalf("what came back cannot be read: %v\n%s", err, Put(p))
			}
			if !reflect.DeepEqual(first, again) {
				t.Errorf("the song changed\n--- back together ---\n%s", Put(p))
			}
		})
	}
}

func TestTheSectionsComeApartWhereTheHeadingsAre(t *testing.T) {
	p, err := Take([]byte("---\ntitle: T\n---\n\n## Verse 1\nla [G]la\n\nmore\n\n## Solo\n```tab\ne|-0-|\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Label: "Verse 1", Body: "la [G]la\n\nmore"},
		{Label: "Solo", Body: "```tab\ne|-0-|\n```"},
	}
	if !reflect.DeepEqual(p.Sections, want) {
		t.Errorf("came apart as %#v", p.Sections)
	}
	if p.Title != "T" {
		t.Errorf("title came back %q", p.Title)
	}
}

// A song with no headings at all is one unlabelled section, which is what the
// parser makes of it and what somebody who pasted a verse in has.
func TestASongWithNoHeadingsIsOneSection(t *testing.T) {
	p, err := Take([]byte("---\ntitle: T\n---\nla la\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sections) != 1 || p.Sections[0].Label != "" || p.Sections[0].Body != "la la" {
		t.Errorf("came apart as %#v", p.Sections)
	}
}

// Editing one section leaves every other one exactly as it was written, which
// rebuilding the whole file from the parse tree could not promise.
func TestEditingOneSectionLeavesTheOthersAlone(t *testing.T) {
	src := []byte("---\ntitle: T\n---\n\n## Verse\nla  [G]la   with   odd    spacing\n\n## Chorus\noh\n")
	p, err := Take(src)
	if err != nil {
		t.Fatal(err)
	}
	p.Sections[1].Body = "ah"
	out := string(Put(p))
	if !strings.Contains(out, "la  [G]la   with   odd    spacing") {
		t.Errorf("the untouched verse was reflowed:\n%s", out)
	}
	if !strings.Contains(out, "ah") {
		t.Errorf("the edit did not land:\n%s", out)
	}
}

// Half a song still goes back together. An editor holds one most of the time,
// and being told about a missing title belongs at the moment of saving.
func TestHalfASongStillGoesBackTogether(t *testing.T) {
	out := string(Put(Parts{Sections: []Part{{Label: "Verse", Body: "la"}}}))
	if !strings.Contains(out, "## Verse") || !strings.Contains(out, "la") {
		t.Errorf("a song without a title came back as:\n%s", out)
	}
}

// A capo half typed survives a redraw rather than being read as nothing.
func TestAHalfTypedCapoSurvives(t *testing.T) {
	if out := string(Put(Parts{Title: "T", Capo: ""})); strings.Contains(out, "capo") {
		t.Errorf("an empty capo was written:\n%s", out)
	}
	if out := string(Put(Parts{Title: "T", Capo: "3"})); !strings.Contains(out, "capo: 3") {
		t.Errorf("a capo of three came back as:\n%s", out)
	}
}

// The same promise against anything the parser will accept, rather than against
// the four songs that happen to be in the tree.
func FuzzASongTakenApartGoesBackTogether(f *testing.F) {
	f.Add("---\ntitle: T\n---\n\n## Verse 1\n[C]hello [x2]\n")
	f.Add("---\ntitle: T\n---\nno headings at all\n")
	f.Add("---\ntitle: T\nnotes: |-\n  one\n  two\n---\n\n## A\nx\n\n## A\ny\n")
	f.Add("---\ntitle: T\n---\n\n## Solo\n```tab\ne|-0-|\n```\n")
	f.Add("---\ntitle: T\n---\n##\nx\n")
	f.Add("---\ntitle: T\n---\n###not a heading\n")

	f.Fuzz(func(t *testing.T, src string) {
		first, err := Parse([]byte(src))
		if err != nil {
			return
		}
		p, err := Take([]byte(src))
		if err != nil {
			t.Fatalf("Parse accepted a song Take would not: %v\ninput: %q", err, src)
		}
		out := Put(p)
		again, err := Parse(out)
		if err != nil {
			t.Fatalf("taken apart and put back, it will not read: %v\ninput: %q\nout: %q",
				err, src, out)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("taking it apart changed the song\ninput: %q\nout: %q", src, out)
		}
	})
}
