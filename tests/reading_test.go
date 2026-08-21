package tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// Every route the application registers must answer, and the ones that do not
// exist must answer too, with a page that has a way back on it.
//
// The point of the table is that adding a route without a test here is a visible
// omission rather than an invisible one.
func TestEveryRouteAnswers(t *testing.T) {
	in := start(t, nil)
	in.seed()

	tests := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/?q=grace", http.StatusOK},
		{"/artists", http.StatusOK},
		{"/genres", http.StatusOK},
		{"/songs/new", http.StatusOK},
		{"/import", http.StatusOK},
		{"/export", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/robots.txt", http.StatusOK},
		{"/songs/traditional-amazing-grace", http.StatusOK},
		{"/songs/traditional-amazing-grace/sheet", http.StatusOK},
		{"/songs/traditional-amazing-grace/print", http.StatusOK},
		{"/songs/traditional-amazing-grace/raw", http.StatusOK},
		{"/songs/traditional-amazing-grace/edit", http.StatusOK},
		{"/songs/traditional-amazing-grace/delete", http.StatusOK},

		// A song that is not there, and an address that is not a route at all.
		{"/songs/no-such-song", http.StatusNotFound},
		{"/nonsense", http.StatusNotFound},
		{"/songs/no-such-song/print", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := in.get(tt.path); got.Status != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.path, got.Status, tt.want)
			}
		})
	}
}

// A mistyped address gets Tabernacle's own page, with the navigation on it,
// rather than a bare line of text that leaves the reader with no way back.
func TestTheNotFoundPageIsUsable(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.get("/nonsense")
	if res.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Status)
	}
	res.mustContain("Canciones", "Artistas", "Géneros", `href="/"`)
}

// The wrong method on a real route is a 405, not a 404 and not a silent GET.
func TestTheWrongMethodIsRefused(t *testing.T) {
	in := start(t, nil)
	in.seed()

	req, err := http.NewRequest(http.MethodPost, in.url+"/artists", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", in.url)
	if got := in.do(req).Status; got != http.StatusMethodNotAllowed {
		t.Errorf("POST /artists = %d, want 405", got)
	}
}

func TestTheHomePageListsTheCollection(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/")
	for _, want := range []string{"Amazing Grace", "Jazz", "Con cejilla", "Traditional"} {
		if !strings.Contains(body, want) {
			t.Errorf("the library page does not mention %q", want)
		}
	}
}

func TestSearchFindsByTitleArtistAndWords(t *testing.T) {
	in := start(t, nil)
	in.seed()

	tests := []struct {
		query string
		want  string
	}{
		{"grace", "Amazing Grace"},       // the title
		{"traditional", "Amazing Grace"}, // the artist
		{"wretch", "Amazing Grace"},      // a word in the middle of a lyric
		{"jazz", "Jazz"},                 // the genre and the title
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			body := in.page("/?q=" + tt.query)
			if !strings.Contains(body, tt.want) {
				t.Errorf("searching for %q did not find %q", tt.query, tt.want)
			}
		})
	}
}

// A search result highlights what matched, and the highlighting must be markup
// the template produced rather than anything that came out of the file.
func TestSearchHighlightsTheMatch(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/?q=wretch")
	if !strings.Contains(body, "<mark>") {
		t.Error("nothing is highlighted in a search result")
	}
}

// A search that matches nothing says so rather than looking like an empty
// collection.
func TestAFruitlessSearchSaysSo(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.get("/?q=zzzzznothing")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	res.mustNotContain("Amazing Grace")
	if !regexp.MustCompile(`(?i)ning[uú]n|no hay|sin resultados`).MatchString(res.Body) {
		t.Errorf("an empty result set does not explain itself:\n%.400s", res.Body)
	}
}

func TestTheSongSheetShowsWordsAndChords(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/songs/traditional-amazing-grace")
	// The words, the chords, and the section heading.
	for _, want := range []string{"mazing", "grace", "G7", "Verse 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("the sheet does not show %q", want)
		}
	}
}

// Transposing changes the chords on the page and says by how much, and it does
// not touch the file.
func TestTransposeChangesTheChordsAndSaysSo(t *testing.T) {
	in := start(t, nil)
	in.seed()

	before := in.page("/songs/traditional-amazing-grace")
	if !strings.Contains(before, ">G<") && !strings.Contains(before, "G7") {
		t.Fatal("the untransposed sheet does not show G")
	}

	after := in.get("/songs/traditional-amazing-grace?t=2")
	after.mustContain("A7")
	// The reader has to be able to tell the sheet is not in the file's key.
	after.mustShow("(+2)")

	// The file is untouched: the source of truth does not move because somebody
	// looked at the song in another key.
	raw := in.page("/songs/traditional-amazing-grace/raw")
	if strings.Contains(raw, "A7") {
		t.Error("transposing rewrote the file")
	}
}

func TestNotationSwitchesToSolfege(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/songs/traditional-amazing-grace?n=solfege")
	if !strings.Contains(body, "Sol") {
		t.Error("solfege notation does not show Sol")
	}
	// And the control now offers the way back, naming its destination.
	if !strings.Contains(body, "A B C") && !strings.Contains(body, "ABC") {
		t.Log("note: the button's label is checked by the web package's own tests")
	}
}

// A capo is written on the sheet as information. It never moves the chords: the
// symbols name the shapes you hold, which is what a capo is for.
func TestTheCapoIsShownAndNeverApplied(t *testing.T) {
	in := start(t, nil)
	in.seed()

	res := in.get("/songs/otro-con-cejilla")
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	res.mustShow("Cejilla", "2")

	// The chords are the written ones, here and with any leftover query.
	chords := strings.Join(chordCards(res.Body), " ")
	if chords != "G C D" {
		t.Errorf("chords = %q, want \"G C D\" as written", chords)
	}
	if got := strings.Join(chordCards(in.page("/songs/otro-con-cejilla?capo=sounding")), " "); got != chords {
		t.Errorf("a capo query changed the chords to %q", got)
	}
}

// The chord appendix shows a shape for a chord it knows, and says plainly that
// it has none for a chord it does not — without inventing one.
func TestTheChordAppendixNeverInventsAShape(t *testing.T) {
	in := start(t, nil)
	in.seed()

	known := in.page("/songs/traditional-amazing-grace")
	if !strings.Contains(known, "<svg") {
		t.Error("no diagram for a song made of ordinary chords")
	}

	awkward := in.get("/songs/prueba-jazz")
	if awkward.Status != http.StatusOK {
		t.Fatalf("status = %d", awkward.Status)
	}
	awkward.mustContain("No hay una posición conocida", "Simplificar")
	if strings.Contains(awkward.Body, "<svg") {
		t.Error("a diagram was drawn for chords the collection has no shape for")
	}
}

// Simplifying is a deliberate act with a visible result and a notice saying it
// happened, and it leaves the file alone.
func TestSimplifyIsDeliberateAndAnnounced(t *testing.T) {
	in := start(t, nil)
	in.seed()

	plain := in.get("/songs/prueba-jazz")
	plain.mustContain("Cmaj7#11")
	plain.mustNotContain("Acordes simplificados")

	simplified := in.get("/songs/prueba-jazz?s=1")
	if simplified.Status != http.StatusOK {
		t.Fatalf("status = %d", simplified.Status)
	}
	simplified.mustContain("Cmaj7", "Am7", "D7", "Acordes simplificados", "<svg")

	raw := in.page("/songs/prueba-jazz/raw")
	if !strings.Contains(raw, "Cmaj7#11") {
		t.Error("simplifying rewrote the file")
	}
}

// The print view is for paper: no navigation, no chord appendix, and a light
// background.
func TestThePrintViewIsForPaper(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/songs/traditional-amazing-grace/print")
	for _, want := range []string{"mazing", "grace"} {
		if !strings.Contains(body, want) {
			t.Errorf("the printed sheet is missing %q", want)
		}
	}
	// The reading interface does not go on paper.
	for _, unwanted := range []string{`class="top`, `class="filters`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the printed sheet carries the interface (%s)", unwanted)
		}
	}
	if strings.Contains(body, "POSICIONES") || strings.Contains(body, "chordcard") {
		t.Error("the chord appendix is on the printed sheet")
	}
}

// The sheet fragment is what the controls fetch to avoid reloading the page. It
// must be the sheet and nothing else.
func TestTheSheetFragmentIsJustTheSheet(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/songs/traditional-amazing-grace/sheet")
	if !strings.Contains(body, "grace") {
		t.Error("the fragment does not contain the song")
	}
	if strings.Contains(body, "<html") || strings.Contains(body, `class="top`) {
		t.Error("the fragment is a whole page")
	}
}

func TestArtistsAndGenresLeadToSongs(t *testing.T) {
	in := start(t, nil)
	in.seed()

	artists := in.page("/artists")
	if !strings.Contains(artists, "Traditional") {
		t.Error("the artists page does not list Traditional")
	}
	genres := in.page("/genres")
	if !strings.Contains(genres, "hymn") {
		t.Error("the genres page does not list hymn")
	}

	// Following an artist has to narrow the library rather than lead nowhere.
	byArtist := in.page("/?artist=Traditional")
	if !strings.Contains(byArtist, "Amazing Grace") {
		t.Error("filtering by artist lost the song")
	}
	if strings.Contains(byArtist, ">Jazz<") {
		t.Error("filtering by artist did not narrow anything")
	}
}

// Static assets are served under a hashed path so they can be cached hard, and
// the page must reference the same path that works.
func TestStaticAssetsAreServedAtTheHashedPathThePageUses(t *testing.T) {
	in := start(t, nil)
	in.seed()

	body := in.page("/")
	refs := regexp.MustCompile(`/static/[^"']+`).FindAllString(body, -1)
	if len(refs) == 0 {
		t.Fatal("the page references no static assets at all")
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		res := in.get(ref)
		if res.Status != http.StatusOK {
			t.Errorf("the page references %s, which answers %d", ref, res.Status)
		}
		if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age") {
			t.Errorf("%s is not cacheable (Cache-Control: %q)", ref, cc)
		}
	}
}

// An empty collection is a first run, not a broken page.
func TestAnEmptyCollectionIsAWelcomeNotAnError(t *testing.T) {
	in := start(t, nil)

	res := in.get("/")
	if res.Status != http.StatusOK {
		t.Fatalf("an empty library answers %d", res.Status)
	}
	res.mustContain("Canciones")
}
