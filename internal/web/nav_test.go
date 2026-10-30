package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/account"
)

var (
	navBlock   = regexp.MustCompile(`(?s)<div class="filters noprint">(.*?)</div>`)
	buttonText = regexp.MustCompile(`btn-quiet[^"]*"[^>]*>([^<]+)<`)
)

func labelsIn(t *testing.T, block *regexp.Regexp, body string) []string {
	t.Helper()
	m := block.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	var out []string
	for _, b := range buttonText.FindAllStringSubmatch(m[1], -1) {
		if s := strings.TrimSpace(b[1]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// The section row names the three ways to walk the collection and the chord
// reference, so it belongs on every page that is a place in it — including the
// error page, because a mistyped address should still offer a way back.
func TestTheSectionRowIsOnEveryPlaceInTheLibrary(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, target := range []string{
		"/",
		"/artists",
		"/genres",
		"/songs/traditional-amazing-grace",
		"/chords",
		"/nope",
	} {
		got := labelsIn(t, navBlock, ts.get(t, target).Body.String())
		if len(got) == 0 {
			t.Errorf("%s has no section row", target)
			continue
		}
		if want := "Canciones|Artistas|Géneros|Acordes"; strings.Join(got, "|") != want {
			t.Errorf("%s section row = %v, want %s", target, got, want)
		}
	}
}

// And it stays off the pages that are an action rather than a place. Offering
// somewhere to go in the middle of writing a song is an invitation to lose it,
// and the library's vocabulary has no business on a login form.
//
// Each of these has its own way out: the editor and the delete confirmation both
// carry a Cancelar, and every page carries the brand link home.
func TestTheSectionRowIsAbsentWhereItWouldNotBelong(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	// /login is asked for signed out, because signed in it redirects home.
	pages := map[string]bool{
		"/songs/new":                              true,
		"/songs/traditional-amazing-grace/edit":   true,
		"/songs/traditional-amazing-grace/delete": true,
		"/settings":                               true,
		"/settings/users":                         true,
		// Import, export and the songbook are settings pages: they carry the
		// settings row instead, because the library's vocabulary is not what
		// somebody needs while looking at them.
		"/import":   true,
		"/songbook": true,
		"/login":    false,
	}
	for target, signedIn := range pages {
		body := ts.get(t, target).Body.String()
		if !signedIn {
			body = ts.getAnon(t, target).Body.String()
		}
		if got := labelsIn(t, navBlock, body); len(got) != 0 {
			t.Errorf("%s carries the section row: %v", target, got)
		}
		// Still not a dead end.
		if !strings.Contains(body, `class="brand" href="/"`) {
			t.Errorf("%s has no way back to the library", target)
		}
	}
}

// The add page offers exactly one control: choosing a file. Anything else there
// is a second way to do the same thing.
func TestAddPageOffersOnlyTheFileChooser(t *testing.T) {
	ts := newTestServer(t, nil)
	body := ts.get(t, "/songs/new").Body.String()

	if !strings.Contains(body, "Cargar archivo") {
		t.Error("the file chooser is missing")
	}
	if strings.Contains(body, "Varias a la vez") {
		t.Error("the bulk import link is still on the add page")
	}
	// The fallback button exists only for a reader with scripting off, so it is
	// inside noscript and not a second button in the row.
	between := body[strings.Index(body, `class="loadbar"`):]
	between = between[:strings.Index(between, "</form>")]
	if strings.Count(between, "<button") != 1 || !strings.Contains(between, "<noscript>") {
		t.Errorf("the load row should hold one label and a noscript fallback:\n%s", between)
	}
}

// Añadir lives in the header, at the far right beside the search, because it is
// an action rather than a place. It is on every page too.
func TestAddSitsInTheHeader(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, target := range []string{"/", "/artists", "/genres", "/songs/new", "/import", "/nope"} {
		body := ts.get(t, target).Body.String()

		header := body[strings.Index(body, `class="top`):]
		header = header[:strings.Index(header, "</header>")]
		if !strings.Contains(header, `href="/songs/new"`) {
			t.Errorf("%s has no Añadir in the header", target)
			continue
		}
		// After the search, so it lands at the right-hand edge.
		if strings.Index(header, `class="search"`) > strings.Index(header, `class="btn add`) {
			t.Errorf("%s puts Añadir before the search box", target)
		}
		// And nowhere else: one way in, not two.
		if strings.Count(body, `href="/songs/new"`) != 1 {
			t.Errorf("%s offers Añadir %d times", target, strings.Count(body, `href="/songs/new"`))
		}
	}
	// It is not one of the sections any more.
	for _, l := range labelsIn(t, navBlock, ts.get(t, "/").Body.String()) {
		if l == "Añadir" {
			t.Error("Añadir is still listed among the sections")
		}
	}
}

// Revisar is gone from the navigation until it belongs to an administrator.
func TestReviewIsNotOfferedInTheNavigation(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, target := range []string{"/", "/artists", "/songs/new"} {
		body := ts.get(t, target).Body.String()
		if strings.Contains(body, ">Revisar<") {
			t.Errorf("%s still offers Revisar in the navigation", target)
		}
	}
	// Importing lives inside Añadir now, not in the navigation.
	for _, label := range labelsIn(t, navBlock, ts.get(t, "/").Body.String()) {
		if label == "Importar" {
			t.Error("Importar is still a section")
		}
	}
}

// The chrome must not offer two labels a reader could mistake for each other.
// "Artista" sitting next to "Artistas" read as the same thing twice and left
// nobody sure which was which, which is why there is one vocabulary now.
func TestSectionLabelsAreDistinct(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	sections := labelsIn(t, navBlock, ts.get(t, "/").Body.String())
	if len(sections) < 4 {
		t.Fatalf("sections = %v", sections)
	}
	for i, a := range sections {
		for j, b := range sections {
			if i >= j {
				continue
			}
			la, lb := strings.ToLower(a), strings.ToLower(b)
			if la == lb {
				t.Errorf("%q appears twice", a)
			}
			if strings.HasPrefix(la, lb) || strings.HasPrefix(lb, la) {
				t.Errorf("%q and %q are too close to sit in the same row", a, b)
			}
		}
	}
}

// There is no ordering row: one vocabulary in the chrome, and the collection is
// alphabetical by default.
func TestNoOrderingRow(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, target := range []string{"/", "/artists", "/genres", "/songs/new"} {
		body := ts.get(t, target).Body.String()
		if strings.Contains(body, "orderlabel") || strings.Contains(body, "filters noprint order") {
			t.Errorf("%s still shows an ordering row", target)
		}
		for _, label := range []string{">A–Z<", ">Recientes<", ">Nuevas<"} {
			if strings.Contains(body, label) {
				t.Errorf("%s still offers the ordering %s", target, label)
			}
		}
	}
	// The parameter still works for a link, it simply has no button.
	if ts.get(t, "/?sort=recent").Code != http.StatusOK {
		t.Error("?sort= stopped working")
	}
}

// Read-only mode has nothing to add, so it does not offer to.
func TestAddIsHiddenInReadOnlyMode(t *testing.T) {
	ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
		"TABERNACLE_READ_ONLY": "true",
	})
	body := ts.get(t, "/").Body.String()
	if strings.Contains(body, `href="/songs/new"`) {
		t.Error("read-only mode still offers Añadir")
	}
	if got := labelsIn(t, navBlock, body); len(got) != 4 {
		t.Errorf("navigation = %v, want the reading sections and the chord reference", got)
	}
	// Reading is untouched.
	if !strings.Contains(body, "Amazing Grace") {
		t.Error("read-only mode broke the library listing")
	}
}

// Loading a file puts its text in the editor and saves nothing. A file whose
// contents you never saw before it landed is how a collection fills with things
// nobody meant to add.
func TestLoadingAFileFillsTheEditorWithoutSaving(t *testing.T) {
	ts := newTestServer(t, nil)

	w := ts.uploadField(t, "file", "/songs/new", "cualquiera.song", []byte(pasted))
	if w.Code != http.StatusOK {
		t.Fatalf("loading = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Loaded into the fields, which is where a song is edited now. A file that
	// would not parse would come back as its text instead, which is where
	// somebody fixing one needs to be.
	if !strings.Contains(body, `name="title" value="Nueva Canción"`) {
		t.Errorf("the file is not in the editor:\n%s", body)
	}
	if !strings.Contains(body, "Cargado: cualquiera.song") {
		t.Error("the page does not say which file arrived")
	}
	// Nothing was written.
	if stats, err := ts.lib.Stats(contextTODO()); err != nil || stats.Songs != 0 {
		t.Errorf("loading a file added %d songs to the collection", stats.Songs)
	}
}

// A chord sheet loads into the fields with its chords moved into the words and
// its title taken from the file's name, to be checked before it is saved.
func TestLoadingAChordSheetFillsTheFields(t *testing.T) {
	ts := newTestServer(t, nil)

	sheet := "Intro: Am G\n\nAm        G\nuna letra inventada\n"
	w := ts.uploadField(t, "file", "/songs/new", "Una prueba.txt", []byte(sheet))
	if w.Code != http.StatusOK {
		t.Fatalf("loading = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`name="title" value="Una prueba"`, `value="Intro"`, "[Am]una letra [G]inventada"} {
		if !strings.Contains(body, want) {
			t.Errorf("the editor has no %q:\n%s", want, body)
		}
	}
	if stats, _ := ts.lib.Stats(contextTODO()); stats.Songs != 0 {
		t.Error("loading a sheet saved it")
	}
}

// Import and export is one of the settings pages, so Ajustes is what is lit in
// the header there, not Añadir.
func TestImportLightsSettingsInTheHeader(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	for target, lit := range map[string]string{"/import": "account", "/songs/new": "add"} {
		body := ts.get(t, target).Body.String()
		header := body[strings.Index(body, `class="top`):]
		header = header[:strings.Index(header, "</header>")]
		for _, button := range []string{"add", "account"} {
			on := strings.Contains(header, `class="btn `+button+` on"`)
			if on != (button == lit) {
				t.Errorf("%s: %s lit = %v", target, button, on)
			}
		}
	}
}

func TestLoadingABrokenFileShowsTheLine(t *testing.T) {
	ts := newTestServer(t, nil)
	broken := []byte("---\ntitle: Roto\ntitel: mal\n---\n")

	w := ts.uploadField(t, "file", "/songs/new", "roto.song", broken)
	if w.Code != http.StatusOK {
		t.Fatalf("loading = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Línea") || !strings.Contains(body, "titel") {
		t.Errorf("the problem is not reported with its line:\n%s", body)
	}
	// The text is still there to be fixed, and nothing was saved.
	if !strings.Contains(body, "titel: mal") {
		t.Error("the loaded text was thrown away")
	}
	if stats, _ := ts.lib.Stats(contextTODO()); stats.Songs != 0 {
		t.Error("a broken file was saved")
	}
}

func TestLoadingRefusesRubbish(t *testing.T) {
	ts := newTestServer(t, nil)

	// Not text at all.
	w := ts.uploadField(t, "file", "/songs/new", "x.song", []byte{0xff, 0xfe, 0x00, 0x01})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a binary file = %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "texto") {
		t.Error("the refusal does not explain itself")
	}

	// Nothing chosen.
	if w := ts.uploadField(t, "file", "/songs/new", "", nil); w.Code != http.StatusBadRequest {
		t.Errorf("an empty upload = %d, want 400", w.Code)
	}

	// From somewhere else.
	if w := ts.uploadForeign(t, "/songs/new", "x.song", []byte(pasted)); w.Code != http.StatusForbidden {
		t.Errorf("a cross-origin load = %d, want 403", w.Code)
	}
}
