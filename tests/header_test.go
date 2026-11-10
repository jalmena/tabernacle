package tests

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The header of a song as lists to choose from.
//
// The key and the tuning are chosen, the artist and the genre are typed with
// what the collection holds on offer. None of it may change what a file says:
// a song that is opened and saved again is saved with the header it had.

// chosen is the value of the option a list has selected, and whether it has one.
func chosen(t *testing.T, page, id string) (string, bool) {
	t.Helper()
	list := regexp.MustCompile(`(?s)<select id="` + id + `"[^>]*>(.*?)</select>`).FindStringSubmatch(page)
	if list == nil {
		t.Fatalf("the page has no list called %s", id)
	}
	m := regexp.MustCompile(`<option value="([^"]*)" selected>`).FindStringSubmatch(list[1])
	if m == nil {
		return "", false
	}
	return m[1], true
}

func header(lines ...string) string {
	return "---\ntitle: Cabecera\n" + strings.Join(lines, "") + "---\n\n## Estrofa\nla [C]la\n"
}

func TestTheTuningIsChosenAndTheUsualOneGoesWithoutSaying(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	for _, c := range []struct {
		name, line string
		want       string // the value the list has chosen
		other      string // what the box beside it holds
	}{
		{"a song that names no tuning", "", "", ""},
		{"one that names the usual one", "tuning: EADGBE\n", "EADGBE", ""},
		{"one that names another in its own words", "tuning: drop d\n", "drop d", ""},
		{"one the list does not hold", "tuning: CGCGCE\n", "custom", "CGCGCE"},
	} {
		in.writeSong("Cabecera.song", header(c.line))
		page := in.page("/songs/cabecera/edit")
		got, _ := chosen(t, page, "f-tuning")
		if got != c.want {
			t.Errorf("%s: the list has chosen %q, want %q", c.name, got, c.want)
		}
		if !strings.Contains(page, `name="tuning" value="`+c.other+`"`) {
			t.Errorf("%s: the box beside the list does not hold %q", c.name, c.other)
		}

		// Sent back as the page holds it, the file is the file it was.
		res := in.post("/songs/cabecera", url.Values{
			"hash": {in.editHash("cabecera")}, "title": {"Cabecera"},
			"tuningpick": {got}, "tuning": {c.other},
			"label0": {"Estrofa"}, "body0": {"la [C]la"},
		})
		if res.Status != 303 {
			t.Fatalf("%s: saving = %d", c.name, res.Status)
		}
		if file := in.file("Cabecera.song"); file != header(c.line) {
			t.Errorf("%s: saved as\n%s", c.name, file)
		}
	}
}

func TestATuningChosenFromTheListIsWrittenByItsName(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	in.writeSong("Cabecera.song", header("tuning: CGCGCE\n"))

	page := in.page("/songs/cabecera/edit")
	for _, want := range []string{`<option value="" >Estándar</option>`, `<option value="Drop D" >Drop D (DADGBE)</option>`,
		`<option value="Eb standard" >Eb standard (Eb Ab Db Gb Bb Eb)</option>`, `<option value="DADGAD" >DADGAD</option>`,
		`<option value="Open D" >Open D (D A D F# A D)</option>`, `<option value="custom" selected>Personalizada…</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the list of tunings has no %s", want)
		}
	}

	// The list wins over whatever is left in the box beside it.
	in.post("/songs/cabecera", url.Values{
		"hash": {in.editHash("cabecera")}, "title": {"Cabecera"},
		"tuningpick": {"Drop D"}, "tuning": {"CGCGCE"},
		"label0": {"Estrofa"}, "body0": {"la [C]la"},
	})
	if file := in.file("Cabecera.song"); file != header("tuning: Drop D\n") {
		t.Errorf("saved as\n%s", file)
	}

	// And the usual one, chosen over another, takes the line away.
	in.post("/songs/cabecera", url.Values{
		"hash": {in.editHash("cabecera")}, "title": {"Cabecera"},
		"tuningpick": {""}, "tuning": {""},
		"label0": {"Estrofa"}, "body0": {"la [C]la"},
	})
	if file := in.file("Cabecera.song"); file != header() {
		t.Errorf("saved as\n%s", file)
	}
}

func TestTheKeyIsChosenAndOneTheListDoesNotHoldIsKept(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")

	for line, want := range map[string]string{"": "", "key: F#m\n": "F#m", "key: Eb\n": "Eb", "key: Gb\n": "Gb", "key: A7\n": "A7"} {
		in.writeSong("Cabecera.song", header(line))
		got, has := chosen(t, in.page("/songs/cabecera/edit"), "f-key")
		if got != want || has != (want != "") {
			t.Errorf("with %q the list has chosen %q", line, got)
		}
	}
	page := in.page("/songs/cabecera/edit")
	for _, want := range []string{`<optgroup label="mayor">`, `<optgroup label="menor">`, `<option value="Bb" >Bb</option>`, `<option value="C#m" >C#m</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the list of keys has no %s", want)
		}
	}
}

func TestTheArtistAndTheGenreOfferWhatTheCollectionHolds(t *testing.T) {
	in := start(t, nil)
	in.claim("jose")
	in.writeSong("Uno.song", "---\ntitle: Uno\nartist: Los Inventados\ngenre: Copla\n---\nla\n")
	in.writeSong("Dos.song", "---\ntitle: Dos\nartist: Ana Nadie\ngenre: zarzuela\n---\nla\n")
	in.writeSong("Tres.song", "---\ntitle: Tres\nartist: Ana Nadie\ngenre: zarzuela\n---\nla\n")

	page := in.page("/songs/new")
	artists := regexp.MustCompile(`(?s)<datalist id="artistnames">(.*?)</datalist>`).FindStringSubmatch(page)
	genres := regexp.MustCompile(`(?s)<datalist id="genrenames">(.*?)</datalist>`).FindStringSubmatch(page)
	if artists == nil || genres == nil {
		t.Fatal("the page offers no artists or no genres")
	}
	if got := strings.Join(regexp.MustCompile(`value="([^"]*)"`).FindAllString(artists[1], -1), " "); got != `value="Ana Nadie" value="Los Inventados"` {
		t.Errorf("the artists on offer are %s", got)
	}
	var names []string
	for _, m := range regexp.MustCompile(`value="([^"]*)"`).FindAllStringSubmatch(genres[1], -1) {
		names = append(names, m[1])
	}
	// The collection's own first, the most used first, each once however it is
	// written; then the usual ones.
	if len(names) < 40 || names[0] != "zarzuela" || names[1] != "Copla" {
		t.Fatalf("the genres on offer begin %v", names[:min(len(names), 4)])
	}
	rest := strings.Join(names[2:], "|")
	for _, want := range []string{"rumba", "balada", "canción de autor", "rock and roll"} {
		if !strings.Contains("|"+rest+"|", "|"+want+"|") {
			t.Errorf("%q is not among the usual genres", want)
		}
	}
	if strings.Contains(strings.ToLower(rest), "copla") {
		t.Error("a genre the collection has is offered a second time")
	}
	if !strings.Contains(page, `name="artist" value="" autocomplete="off"`) || !strings.Contains(page, `list="artistnames"`) || !strings.Contains(page, `list="genrenames"`) {
		t.Error("the two boxes are not joined to what they offer")
	}
}
