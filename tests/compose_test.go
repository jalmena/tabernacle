package tests

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Adding a song by filling in fields rather than by typing a file.
//
// Somebody adding a song should not have to know the format by heart, and every
// part of this has to work with no script at all: adding a section, throwing one
// away and moving one are ordinary form submissions that come back with the
// change made.

func TestASongCanBeAddedByFillingInFields(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	// The empty form offers the sections a song is made of.
	page := in.page("/songs/new")
	for _, kind := range []string{"intro", "verse", "chorus", "solo"} {
		if !strings.Contains(page, `<option value="`+kind+`">`) {
			t.Errorf("the form does not offer to add a %s", kind)
		}
	}

	// A verse, typed in.
	form := url.Values{
		"title": {"Prueba de campos"}, "artist": {"Nadie"},
		"key": {"G"}, "capo": {"2"},
		"add": {"verse"}, "addnow": {"1"},
	}
	res := in.post("/songs", form)
	if res.Status != http.StatusOK {
		t.Fatalf("adding a section = %d, want 200", res.Status)
	}
	if !strings.Contains(res.Body, `name="label0"`) || !strings.Contains(res.Body, `name="body0"`) {
		t.Fatal("the section did not come back on the page")
	}
	if !strings.Contains(res.Body, `value="Prueba de campos"`) {
		t.Error("what had been typed was lost when the section was added")
	}

	// And a solo, which arrives with a grid already drawn.
	form.Set("label0", "Estrofa")
	form.Set("body0", "la [G]la")
	form.Set("add", "solo")
	form.Set("addnow", "1")
	res = in.post("/songs", form)
	if res.Status != http.StatusOK {
		t.Fatalf("adding a solo = %d, want 200", res.Status)
	}
	if !strings.Contains(res.Body, "```tab") || !strings.Contains(res.Body, "e|---") {
		t.Error("a solo did not come with an empty grid to fill in")
	}

	// Saved.
	form.Del("add")
	form.Del("addnow")
	form.Set("label1", "Solo")
	form.Set("body1", "```tab\ne|-0-|\nB|----|\nG|----|\nD|----|\nA|----|\nE|----|\n```")
	res = in.post("/songs", form)
	if res.Status != http.StatusSeeOther {
		t.Fatalf("saving = %d, want a redirect\n%s", res.Status, res.Body)
	}

	sheet := in.page(res.Header.Get("Location"))
	for _, want := range []string{"Prueba de campos", "Nadie", "Estrofa", "Solo"} {
		if !strings.Contains(sheet, want) {
			t.Errorf("the saved song does not show %q", want)
		}
	}
}

func TestASectionCanBeThrownAwayAndMoved(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	form := url.Values{
		"title":  {"Tres partes"},
		"label0": {"Intro"}, "body0": {"uno"},
		"label1": {"Estrofa"}, "body1": {"dos"},
		"label2": {"Coro"}, "body2": {"tres"},
	}

	moved := form
	moved.Set("up1", "1")
	res := in.post("/songs", moved)
	if res.Status != http.StatusOK {
		t.Fatalf("moving = %d, want 200", res.Status)
	}
	if a, b := labels(res.Body); a != "Estrofa" || b != "Intro" {
		t.Errorf("after moving the second up, the first two are %q and %q", a, b)
	}
	moved.Del("up1")

	gone := form
	gone.Set("del1", "1")
	res = in.post("/songs", gone)
	if res.Status != http.StatusOK {
		t.Fatalf("removing = %d, want 200", res.Status)
	}
	if strings.Contains(res.Body, `value="Estrofa"`) {
		t.Error("the section that was thrown away is still there")
	}
	if !strings.Contains(res.Body, `value="Coro"`) {
		t.Error("the section after it went too")
	}
}

// The file is still reachable, and the two ways round agree about the song.
func TestTheFileIsStillThereBehindTheFields(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	form := url.Values{
		"title": {"Ida y vuelta"}, "key": {"Am"},
		"label0": {"Estrofa"}, "body0": {"la [Am]la"},
		"plainer": {"1"},
	}
	res := in.post("/songs", form)
	if res.Status != http.StatusOK {
		t.Fatalf("asking for the file = %d, want 200", res.Status)
	}
	for _, want := range []string{"title: Ida y vuelta", "key: Am", "## Estrofa", "la [Am]la"} {
		if !strings.Contains(res.Body, want) {
			t.Errorf("the file the fields stand for is missing %q\n%s", want, res.Body)
		}
	}

	// And back again, unchanged.
	back := url.Values{
		"plain":   {"1"},
		"text":    {"---\ntitle: Ida y vuelta\nkey: Am\n---\n\n## Estrofa\nla [Am]la\n"},
		"fielder": {"1"},
	}
	res = in.post("/songs", back)
	if res.Status != http.StatusOK {
		t.Fatalf("going back to the fields = %d, want 200", res.Status)
	}
	if !strings.Contains(res.Body, `value="Ida y vuelta"`) || !strings.Contains(res.Body, `value="Estrofa"`) {
		t.Errorf("the song did not come back as fields\n%s", res.Body)
	}
}

var labelRe = regexp.MustCompile(`name="label\d+" value="([^"]*)"`)

func labels(body string) (string, string) {
	m := labelRe.FindAllStringSubmatch(body, -1)
	if len(m) < 2 {
		return "", ""
	}
	return m[0][1], m[1][1]
}

// Looking at the file and coming back does not demand a title first.
//
// An editor holds a half-written song most of the time. Refusing to show one as
// fields until it had a title meant somebody who glanced at the file was told to
// name their song before they could carry on typing it.
func TestGoingToTheFileAndBackDoesNotDemandATitle(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"plain":   {"1"},
		"text":    {"---\nartist: Nadie\n---\n\n## Estrofa\nla la\n"},
		"fielder": {"1"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("going back to the fields = %d, want 200", res.Status)
	}
	if !strings.Contains(res.Body, `name="artist" value="Nadie"`) {
		t.Errorf("a song without a title did not come back as fields\n%s", res.Body)
	}
	if !strings.Contains(res.Body, `value="Estrofa"`) {
		t.Error("its section did not come back either")
	}
}
