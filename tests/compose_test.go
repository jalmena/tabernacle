package tests

import (
	"net/http"
	"net/url"
	"reflect"
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
	// The names of the sections, not the page's whole text: the list of names
	// offered for typing holds every kind of section whatever the song has.
	if got := sectionsIn(res.Body); !reflect.DeepEqual(got, []string{"Intro", "Coro"}) {
		t.Errorf("after throwing away the second, the sections are %v", got)
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

// A chord sheet pasted into the file comes back as fields with its chords moved
// into the words, since that is how a song copied off a page arrives.
func TestAPastedChordSheetComesBackAsFields(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"plain":   {"1"},
		"text":    {"ESTRIBILLO:\nAm        G\nuna letra inventada\n"},
		"fielder": {"1"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("going to the fields = %d, want 200", res.Status)
	}
	if !strings.Contains(res.Body, `value="Estribillo"`) {
		t.Errorf("the section was not recognised\n%s", res.Body)
	}
	if !strings.Contains(res.Body, "[Am]una letra [G]inventada") {
		t.Errorf("the chords did not move into the words\n%s", res.Body)
	}
	// Still the add page, so Añadir is still the lit button.
	if !strings.Contains(res.Body, `class="btn add on"`) {
		t.Error("Añadir is not lit after a round trip in the editor")
	}
}

// Ticked sections join into one where the first stood, under the name most of
// them share, or the first one's when none has a majority.
func TestTickedSectionsJoin(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Juntas"},
		"label0": {"Estrofa"}, "body0": {"uno"},
		"label1": {"Coro"}, "body1": {"dos"},
		"label2": {"Estrofa"}, "body2": {"tres"},
		"label3": {"Puente"}, "body3": {"cuatro"},
		"pick0": {"1"}, "pick1": {"1"}, "pick2": {"1"},
		"merge": {"1"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("joining = %d, want 200", res.Status)
	}
	if got := sectionsIn(res.Body); !reflect.DeepEqual(got, []string{"Estrofa", "Puente"}) {
		t.Errorf("sections after joining = %v", got)
	}
	if !strings.Contains(res.Body, "uno\n\ndos\n\ntres") {
		t.Errorf("the joined text is not one after the other\n%s", res.Body)
	}

	// A tie goes to the first.
	res = in.post("/songs", url.Values{
		"title":  {"Juntas"},
		"label0": {"Coro"}, "body0": {"a"},
		"label1": {"Estrofa"}, "body1": {"b"},
		"pick0": {"1"}, "pick1": {"1"}, "merge": {"1"},
	})
	if got := sectionsIn(res.Body); !reflect.DeepEqual(got, []string{"Coro"}) {
		t.Errorf("a tie went to %v, want the first", got)
	}
}

// A section splits in two before a line, both halves keeping its name, and the
// caret goes to the second half.
func TestASectionSplitsAtALine(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	res := in.post("/songs", url.Values{
		"title":  {"Partida"},
		"label0": {"Estrofa"}, "body0": {"uno\ndos\n\ntres"},
		"split0": {"1"}, "at": {"2"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("splitting = %d, want 200", res.Status)
	}
	if got := sectionsIn(res.Body); !reflect.DeepEqual(got, []string{"Estrofa", "Estrofa"}) {
		t.Errorf("sections after splitting = %v", got)
	}
	if !regexp.MustCompile(`name="body1"[^>]*autofocus`).MatchString(res.Body) {
		t.Error("the caret is not put in the second half")
	}
}

// Buttons that change nothing still do not save: ↑ on the first section, and
// Enter in one of the fields.
func TestANoOpIsNotASave(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	for name, extra := range map[string]url.Values{
		"up on the first": {"up0": {"1"}},
		"enter":           {"stay": {"1"}},
	} {
		form := url.Values{"title": {"Quieta"}, "label0": {"Estrofa"}, "body0": {"la la"}}
		for k, v := range extra {
			form[k] = v
		}
		if res := in.post("/songs", form); res.Status != http.StatusOK {
			t.Errorf("%s answered %d, want the editor back", name, res.Status)
		}
	}
	if files := in.songFiles(); len(files) != 0 {
		t.Errorf("something was saved: %v", files)
	}
}

// A new section goes after the one the caret is in when the page says which,
// and at the end when it does not, as it must without a script.
func TestANewSectionGoesWhereTheCaretIs(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	form := url.Values{
		"title":  {"Orden"},
		"label0": {"Intro"}, "body0": {"a"},
		"label1": {"Final"}, "body1": {"b"},
		"add": {"chorus"}, "addnow": {"1"}, "after": {"0"},
	}
	if got := sectionsIn(in.post("/songs", form).Body); !reflect.DeepEqual(got, []string{"Intro", "Estribillo", "Final"}) {
		t.Errorf("after the first, sections = %v", got)
	}
	form.Del("after")
	if got := sectionsIn(in.post("/songs", form).Body); !reflect.DeepEqual(got, []string{"Intro", "Final", "Estribillo"}) {
		t.Errorf("with nowhere said, sections = %v", got)
	}
}

// sectionsIn lists the section names an editor page holds, in order.
func sectionsIn(body string) []string {
	var out []string
	for _, m := range labelRe.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
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

// A page that holds something no file has says so, which is what lets it ask
// before it is left: one that came back from a button, and one a file was loaded
// into. A page that was only opened does not.
func TestThePageSaysWhenItHoldsSomethingUnsaved(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	in.writeSong("Quieta.song", "---\ntitle: Quieta\n---\n\n## Estrofa\nla [C]la\n")

	for _, path := range []string{"/songs/new", "/songs/quieta/edit"} {
		if strings.Contains(in.page(path), "data-unsaved") {
			t.Errorf("%s says it holds something unsaved before anything was done", path)
		}
	}

	res := in.post("/songs/quieta", url.Values{
		"hash":   {in.editHash("quieta")},
		"title":  {"Quieta"},
		"label0": {"Estrofa"}, "body0": {"la [C]la"},
		"add": {"chorus"}, "addnow": {"1"},
	})
	res.mustContain(`class="editor" data-unsaved`)

	res = in.upload("/songs/new", "file", map[string][]byte{"Otra.song": []byte("---\ntitle: Otra\n---\nla\n")})
	res.mustContain(`class="editor" data-unsaved`)

	// And both ways of not wanting it are marked as such.
	if n := strings.Count(in.page("/songs/quieta/edit"), "data-discard"); n != 1 {
		t.Errorf("%d ways to cancel are marked on the page, want the one", n)
	}
}
