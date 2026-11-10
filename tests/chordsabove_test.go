package tests

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/song"
)

// Editing a song with its chords above its words.
//
// The script that draws them is not run here. What these check is the part it
// stands on: every section reaches the page already taken apart, a section sent
// back as words and positions is written with its brackets, and one that nobody
// touched is saved as the text it was.

const hymn = `---
title: Himno
---

## Estrofa
A[G]mazing grace how [C]sweet [x2]

## Solo
` + "```tab\ne|-0-|\n```\n"

var rowsField = regexp.MustCompile(`name="rows(\d+)" value="([^"]*)"\s+autocomplete="off" disabled`)

// rowsOn reads the sections an editor page hands to the script.
func rowsOn(t *testing.T, body string) map[string][]song.Row {
	t.Helper()
	out := map[string][]song.Row{}
	for _, m := range rowsField.FindAllStringSubmatch(body, -1) {
		var rows []song.Row
		if err := json.Unmarshal([]byte(html.UnescapeString(m[2])), &rows); err != nil {
			t.Fatalf("section %s is not handed over as rows: %v", m[1], err)
		}
		out[m[1]] = rows
	}
	return out
}

func (in *instance) file(rel string) string {
	in.t.Helper()
	raw, err := os.ReadFile(filepath.Join(in.songs, rel))
	if err != nil {
		in.t.Fatal(err)
	}
	return string(raw)
}

func TestTheEditorHandsEachSectionOverTakenApart(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	in.writeSong("Himno.song", hymn)

	page := in.page("/songs/himno/edit")
	got := rowsOn(t, page)
	want := map[string][]song.Row{
		"0": {{Words: "Amazing grace how sweet ", Marks: []song.Mark{
			{At: 1, Text: "G", Chord: true}, {At: 18, Text: "C", Chord: true}, {At: 24, Text: "x2"},
		}}},
		"1": {{Words: "e|-0-|", Fenced: true, Info: "tab"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("handed over as\n%+v\nwant\n%+v", got, want)
	}
	// Beside the text it was, which is what goes back when nobody touches it and
	// what there is to edit with no script.
	if !strings.Contains(page, "A[G]mazing grace how [C]sweet [x2]</textarea>") {
		t.Error("the section is not also there as text")
	}
	for _, piece := range []string{`<template id="chordkit">`, `id="chordpick"`, `class="chip"`} {
		if !strings.Contains(page, piece) {
			t.Errorf("the page has no %s for the script to build with", piece)
		}
	}
}

func TestASectionSentAsWordsAndPlacesIsWrittenWithItsBrackets(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	rows := `[{"words":"Amazing grace [sic]","marks":[{"at":8,"text":"C","chord":true},{"at":1,"text":"G"},{"at":19,"text":"x2"}]},
		{"words":""},
		{"words":"e|-0-|","fenced":true,"info":"tab"}]`
	res := in.post("/songs", url.Values{
		"title":  {"Movida"},
		"label0": {"Estrofa"}, "rows0": {rows},
		// What the box held before the chords were moved, which is not sent
		// once they have been and must lose if it is.
		"body0": {"[G]Amazing grace"},
	})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("saving = %d, want a redirect\n%s", res.Status, res.Body)
	}
	want := "---\ntitle: Movida\n---\n\n## Estrofa\nA[G]mazing [C]grace \\[sic][x2]\n\n```tab\ne|-0-|\n```\n"
	if got := in.file("Movida.song"); got != want {
		t.Errorf("saved as\n%s\nwant\n%s", got, want)
	}
}

// A section nobody moved a chord in goes back as the text it was, so it is
// saved byte for byte, however it was written.
func TestASectionNobodyTouchedIsSavedAsItWas(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	loose := "la [ Am ]la \\a"
	in.writeSong("Suelta.song", "---\ntitle: Suelta\n---\n\n## Uno\n"+loose+"\n\n## Dos\n[C]la\n")

	res := in.post("/songs/suelta", url.Values{
		"hash":   {in.editHash("suelta")},
		"title":  {"Suelta"},
		"label0": {"Uno"}, "body0": {loose},
		"label1": {"Dos"}, "rows1": {`[{"words":"la","marks":[{"at":2,"text":"C"}]}]`},
	})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("saving = %d, want a redirect\n%s", res.Status, res.Body)
	}
	want := "---\ntitle: Suelta\n---\n\n## Uno\n" + loose + "\n\n## Dos\nla[C]\n"
	if got := in.file("Suelta.song"); got != want {
		t.Errorf("saved as\n%q\nwant\n%q", got, want)
	}
}

// The buttons that go to the server and come back work on a section whichever
// way it was sent.
func TestASectionSentAsWordsAndPlacesCanStillBeDivided(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Partida"},
		"label0": {"Estrofa"},
		"rows0":  {`[{"words":"uno","marks":[{"at":0,"text":"G"}]},{"words":"dos","marks":[{"at":3,"text":"D"}]}]`},
		"split0": {"1"}, "at": {"1"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("dividing = %d, want the editor back", res.Status)
	}
	res.mustContain(`name="body0"`, ">[G]uno</textarea>", `name="body1"`, ">dos[D]</textarea>")
	if got := rowsOn(t, res.Body); len(got) != 2 || got["1"][0].Words != "dos" {
		t.Errorf("the halves are handed back as %+v", got)
	}
}

// What cannot be taken apart is left as text, with the mistake in it to mend.
func TestASectionThatCannotBeTakenApartStaysABox(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Rota"},
		"label0": {"Bien"}, "body0": {"la [C]la"},
		"label1": {"Mal"}, "body1": {"la [C la"},
		"stay": {"1"},
	})
	got := rowsOn(t, res.Body)
	if _, ok := got["0"]; !ok {
		t.Error("the section that reads was not handed over")
	}
	if _, ok := got["1"]; ok {
		t.Error("the section with a bracket left open was handed over")
	}
	res.mustContain(`name="body1"`)
}

// Rows that do not read are not a section: what the box held stands.
func TestRowsThatDoNotReadAreIgnored(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Ruido"},
		"label0": {"Estrofa"}, "body0": {"la [C]la"}, "rows0": {"{not rows"},
	})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("saving = %d, want a redirect", res.Status)
	}
	if got := in.file("Ruido.song"); !strings.HasSuffix(got, "## Estrofa\nla [C]la\n") {
		t.Errorf("saved as\n%s", got)
	}
}

// And with nothing beside them to stand for the section, nothing is saved at
// all: an empty section where there was a verse is worse than being told.
func TestRowsThatDoNotReadAndNothingElseAreRefused(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	in.writeSong("Himno.song", hymn)

	res := in.post("/songs/himno", url.Values{
		"hash":   {in.editHash("himno")},
		"title":  {"Himno"},
		"label0": {"Estrofa"}, "rows0": {"{not rows"},
		"label1": {"Solo"}, "body1": {"```tab\ne|-0-|\n```"},
	})
	if res.Status != http.StatusBadRequest {
		t.Fatalf("saving = %d, want it refused", res.Status)
	}
	if got := in.file("Himno.song"); got != hymn {
		t.Errorf("the file was changed:\n%s", got)
	}
}

// A name typed where a chord goes and saved before anything had read it is read
// then, the way a typed name is.
func TestANameSavedBeforeItWasReadIsReadOnTheWayIn(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Deprisa"},
		"label0": {"Estrofa"},
		"rows0":  {`[{"words":"la la la","marks":[{"at":0,"text":"lam","typed":true},{"at":3,"text":"x2","typed":true},{"at":6,"text":"lam"}]}]`},
	})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("saving = %d, want a redirect\n%s", res.Status, res.Body)
	}
	if got := in.file("Deprisa.song"); !strings.HasSuffix(got, "## Estrofa\n[Am]la [x2]la [lam]la\n") {
		t.Errorf("saved as\n%s", got)
	}
}

// The chooser is asked what a typed name means, and answers with what it could
// be changed into. Anybody may ask: it is a reference, and it changes nothing.
func TestTheChooserSaysWhatWasTypedAndWhatItCouldBecome(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	anon := in.anon()

	res := anon.get("/chords/pick?c=" + url.QueryEscape("lam7/sol"))
	if res.Status != http.StatusOK {
		t.Fatalf("asking = %d", res.Status)
	}
	res.mustContain(
		`data-symbol="Am7/G"`,
		`data-c="Dm7/C"`,       // the whole chord on another note
		`<option value="Am/G"`, // another kind on the same note and bass
		`<option value="Am7/G" selected>Am7 · séptima menor`,
		`<option value="Am7/G" selected>G<`, // the bass it has
		`<option value="Am7" >la fundamental`,
	)
	res.mustNotContain("<html", "<header")
}

func TestTheChooserOffersOnlyANoteForWhatIsNotAChord(t *testing.T) {
	in := start(t, nil)

	for _, typed := range []string{"", "x2", "let ring", strings.Repeat("A", 200)} {
		res := in.get("/chords/pick?c=" + url.QueryEscape(typed))
		if res.Status != http.StatusOK {
			t.Fatalf("%q answered %d", typed, res.Status)
		}
		res.mustContain(`data-symbol=""`, `data-c="C"`, `data-c="Bb"`)
		res.mustNotContain("<select")
	}
}
