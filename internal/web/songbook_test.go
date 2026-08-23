package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/account"
)

// threeSongs is a collection with more than one artist and more than one genre,
// so a filter has something to narrow.
func threeSongs() map[string]string {
	return map[string]string{
		"Traditional/Amazing Grace.song": graceSong,
		"Otro/Segunda.song": "---\ntitle: Segunda\nartist: Otro\nkey: C\ngenre: folk\n---\n\n" +
			"## Estrofa 1\n[C]uno [G]dos\n",
	}
}

var songbookSection = regexp.MustCompile(`class="songbook-song[^"]*"`)

func bookSongs(body string) int { return len(songbookSection.FindAllString(body, -1)) }

// Three songs chosen by hand come out as one document with three sheets, in the
// order the listing had them.
func TestASongbookHoldsTheSongsChosen(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	w := ts.get(t, "/songbook/print?s=traditional-amazing-grace&s=otro-segunda&index=1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if got := bookSongs(body); got != 2 {
		t.Errorf("the book holds %d songs, want 2", got)
	}
	mustHave(t, body, "Amazing Grace", "Segunda", "songbook-index")
	// A book is for paper: no interface, and sixty chord appendices are not a
	// songbook.
	mustNotHave(t, body, `class="top`, `class="filters noprint"`, "chordcard")
}

// The book's order is the listing's order, which is what the sort parameter is
// for.
func TestASongbookKeepsTheOrderItWasGiven(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	body := ts.get(t, "/songbook/print?s=otro-segunda&s=traditional-amazing-grace").Body.String()
	first, second := strings.Index(body, "Segunda"), strings.Index(body, "Amazing Grace")
	if first < 0 || second < 0 {
		t.Fatal("both songs should be present")
	}
	if first > second {
		t.Error("the songs came out in a different order from the one asked for")
	}
}

// Duplicates are dropped rather than printed twice.
func TestASongbookPrintsEachSongOnce(t *testing.T) {
	ts := newTestServer(t, threeSongs())
	body := ts.get(t, "/songbook/print?s=traditional-amazing-grace&s=traditional-amazing-grace").Body.String()
	if got := bookSongs(body); got != 1 {
		t.Errorf("the book holds %d copies, want 1", got)
	}
}

// The filter form is how "print every hymn" stays a short address.
func TestASongbookCanBeDefinedByAFilter(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	body := ts.get(t, "/songbook/print?all=1&genre=hymn").Body.String()
	if got := bookSongs(body); got != 1 {
		t.Errorf("filtering by genre gave %d songs, want 1", got)
	}
	mustHave(t, body, "Amazing Grace")
	mustNotHave(t, body, "Segunda")
}

// A stale bookmark prints what it still can and says what it could not, which is
// more use than a 404.
func TestASongbookSkipsSongsThatHaveGone(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	w := ts.get(t, "/songbook/print?s=traditional-amazing-grace&s=ya-no-existe")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if got := bookSongs(body); got != 1 {
		t.Errorf("the book holds %d songs, want 1", got)
	}
	if !strings.Contains(body, "ya no está en la") {
		t.Error("the book does not say that a song was skipped")
	}
}

// Nothing left to print is a refusal that explains itself, not an empty page.
func TestASongbookOfNothingSaysSo(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	if got := ts.get(t, "/songbook/print?s=ya-no-existe").Code; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
	// And no songs at all goes back to the chooser rather than printing a blank.
	if got := ts.get(t, "/songbook/print").Code; got != http.StatusSeeOther {
		t.Errorf("an empty selection = %d, want a redirect to the chooser", got)
	}
}

// The explicit form has a cap, and the refusal names the way round it.
func TestASongbookRefusesMoreThanItsCap(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	// Distinct slugs, or the deduplication would leave one.
	parts := make([]string, 0, maxSongbookSongs+1)
	for i := range maxSongbookSongs + 1 {
		parts = append(parts, "s=song-"+strconv.Itoa(i))
	}
	w := ts.get(t, "/songbook/print?"+strings.Join(parts, "&"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	mustHave(t, w.Body.String(), "Demasiadas canciones")
	if !strings.Contains(w.Body.String(), "esta búsqueda") {
		t.Error("the refusal does not point at the filter form")
	}
}

// Each song prints in the key its reader asked to remember, which is what
// "Recordar el tono" is for.
func TestASongbookHonoursEachSavedKey(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	if w := ts.post(t, "/songs/traditional-amazing-grace/transpose", url.Values{"t": {"2"}}); w.Code >= 400 {
		t.Fatalf("remembering a key = %d", w.Code)
	}
	body := ts.get(t, "/songbook/print?s=traditional-amazing-grace").Body.String()
	// G C Em D two semitones up.
	for _, want := range []string{">A<", ">D<", ">F#m<", ">E<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the book is missing %s, so it did not use the remembered key", want)
		}
	}
	if strings.Contains(body, ">Em<") {
		t.Error("the book still shows the written key")
	}
}

// The chooser is reading, so it needs no account.
func TestTheSongbookIsPublic(t *testing.T) {
	ts := newTestServer(t, threeSongs())
	ts.account(t, "admin", account.RoleAdmin)

	for _, path := range []string{"/songbook", "/songbook/print?s=traditional-amazing-grace"} {
		if got := ts.getAnon(t, path).Code; got != http.StatusOK {
			t.Errorf("GET %s = %d for a stranger, want 200", path, got)
		}
	}
}

// The chooser works with no scripting: the boxes are a plain GET form, and the
// buttons that need JavaScript are the two conveniences.
func TestTheSongbookChooserWorksWithoutScripting(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	body := ts.get(t, "/songbook").Body.String()
	mustHave(t, body,
		`action="/songbook/print" method="get"`,
		`type="checkbox" name="s"`,
		`type="submit"`,
	)
	// The script is loaded by file, never inline.
	if strings.Contains(body, "<script>") {
		t.Error("the page carries an inline script")
	}
}

// longSong is a song no one side of paper will hold.
func longSong() string {
	var b strings.Builder
	b.WriteString("---\ntitle: Larga\nartist: Otro\nkey: C\n---\n")
	for v := 1; v <= 8; v++ {
		b.WriteString("\n## Estrofa " + strconv.Itoa(v) + "\n")
		for i := 0; i < 4; i++ {
			b.WriteString("[C]una linea con acordes y bastante texto encima\n")
			b.WriteString("[F]y otra linea igual de larga para hacer bulto\n")
		}
	}
	return b.String()
}

// A song too long for one side is set in two columns; a short one is not.
//
// Two columns hold about twice as much, which is the difference between a song
// you can see whole and one you have to turn over in the middle of a verse.
func TestALongSongIsSetInTwoColumns(t *testing.T) {
	ts := newTestServer(t, map[string]string{
		"Traditional/Amazing Grace.song": graceSong,
		"Otro/Larga.song":                longSong(),
	})

	short := ts.get(t, "/songbook/print?s=traditional-amazing-grace").Body.String()
	if strings.Contains(short, "twocol") {
		t.Error("a song that fits one side was set in two columns")
	}

	long := ts.get(t, "/songbook/print?s=otro-larga").Body.String()
	if !strings.Contains(long, `class="songbook-song twocol"`) {
		t.Error("a song too long for one side was not set in two columns")
	}
}

// A heading never leaves its stanza behind.
//
// Asserted on the markup rather than on a stylesheet rule, because the rule that
// would express it — break-after: avoid — is honoured unevenly by print engines
// and hardly at all inside columns. Wrapping the two in one box is the guarantee.
func TestAHeadingTravelsWithItsStanza(t *testing.T) {
	ts := newTestServer(t, threeSongs())
	body := ts.get(t, "/songbook/print?s=otro-segunda").Body.String()

	opening := regexp.MustCompile(`(?s)<div class="opening">(.*?)</div>\s*</section>`)
	m := opening.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the heading and its stanza are not wrapped together")
	}
	if !strings.Contains(m[1], "<h2>Estrofa 1</h2>") {
		t.Error("the wrapper does not hold the heading")
	}
	if !strings.Contains(m[1], `class="block"`) {
		t.Error("the wrapper does not hold the stanza")
	}
}

// Printing opens a document beside the page rather than replacing it: a reader
// who prints has not finished with the song they were looking at.
func TestPrintingOpensInANewTab(t *testing.T) {
	ts := newTestServer(t, threeSongs())

	song := ts.get(t, "/songs/traditional-amazing-grace").Body.String()
	printLink := regexp.MustCompile(`<a[^>]*href="[^"]*/print[^"]*"[^>]*>`)
	m := printLink.FindString(song)
	if m == "" {
		t.Fatal("the song page offers no way to print")
	}
	if !strings.Contains(m, `target="_blank"`) || !strings.Contains(m, `rel="noopener"`) {
		t.Errorf("the print link does not open a new tab safely: %s", m)
	}

	book := ts.get(t, "/songbook").Body.String()
	form := regexp.MustCompile(`<form[^>]*action="/songbook/print"[^>]*>`).FindString(book)
	if form == "" {
		t.Fatal("the songbook has no print form")
	}
	if !strings.Contains(form, `target="_blank"`) {
		t.Errorf("the songbook does not print into a new tab: %s", form)
	}
}
