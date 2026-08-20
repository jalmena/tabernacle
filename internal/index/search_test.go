package index

import (
	"context"
	"strings"
	"testing"
)

// seed fills an index with songs that give the search tests something real to
// match against.
func seed(t *testing.T, x *Index) {
	t.Helper()
	ctx := context.Background()
	songs := []Record{
		{
			RelPath: "Traditional/Amazing Grace.song", Title: "Amazing Grace", Artist: "Traditional",
			Key: "G", Tags: []string{"hymn", "public-domain"}, Chords: []string{"G", "C", "Em", "D"},
			Lyrics: "Amazing grace how sweet the sound that saved a wretch like me",
			Notes:  "Words by John Newton",
		},
		{
			RelPath: "Joaquín Sabina/Y nos dieron las diez.song", Title: "Y nos dieron las diez",
			Artist: "Joaquín Sabina", Key: "Am", Tags: []string{"cancion", FavoriteTag},
			Chords: []string{"Am", "F", "C", "G"},
			Lyrics: "Mi corazón está partido y así nos dieron las diez y las once",
		},
		{
			RelPath: "Bill Withers/Ain't No Sunshine.song", Title: "Ain't No Sunshine",
			Artist: "Bill Withers", Key: "Am", Tags: []string{"soul"},
			Chords: []string{"Am", "Em", "G", "A7"},
			Lyrics: "Ain't no sunshine when she's gone, it's not warm when she's away",
		},
	}
	for _, r := range songs {
		r.Slug = SlugForPath(r.RelPath)
		r.Hash = strings.Repeat("a", 63) + string("012"[len(r.Title)%3])
		if _, err := x.Upsert(ctx, r); err != nil {
			t.Fatalf("seed %s: %v", r.RelPath, err)
		}
	}
}

func titles(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Title
	}
	return out
}

func TestSearchAcrossEveryColumn(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	tests := []struct {
		name, text, want string
	}{
		{"title", "amazing", "Amazing Grace"},
		{"artist", "withers", "Ain't No Sunshine"},
		{"lyrics", "wretch", "Amazing Grace"},
		{"notes", "newton", "Amazing Grace"},
		{"tag", "soul", "Ain't No Sunshine"},
		{"chord", "A7", "Ain't No Sunshine"},
		{"prefix", "sunsh", "Ain't No Sunshine"},
		{"two terms", "amazing grace", "Amazing Grace"},
		{"apostrophe is a separator", "ain't", "Ain't No Sunshine"},
		{"stem of a contraction", "ain", "Ain't No Sunshine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, total, err := x.List(ctx, Query{Text: tt.text})
			if err != nil {
				t.Fatalf("List(%q): %v", tt.text, err)
			}
			if total == 0 {
				t.Fatalf("searching %q found nothing", tt.text)
			}
			if hits[0].Title != tt.want {
				t.Errorf("searching %q ranked %v first, want %q", tt.text, titles(hits), tt.want)
			}
		})
	}
}

// The reason full-text search is worth a dependency at this scale: a Spanish
// collection has to be searchable without typing the accents.
func TestSearchIgnoresAccents(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	for _, text := range []string{"corazon", "corazón", "CORAZON", "Joaquin", "Joaquín", "cancion"} {
		hits, total, err := x.List(ctx, Query{Text: text})
		if err != nil {
			t.Fatalf("List(%q): %v", text, err)
		}
		if total == 0 {
			t.Errorf("searching %q found nothing", text)
			continue
		}
		if hits[0].Title != "Y nos dieron las diez" {
			t.Errorf("searching %q ranked %v first", text, titles(hits))
		}
	}
}

// A title match should beat a match buried in a fourth verse.
func TestSearchRanksTitlesAboveLyrics(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	ctxSeed := []Record{
		{RelPath: "A/Sunshine.song", Title: "Sunshine", Artist: "A", Lyrics: "nothing relevant here"},
		{RelPath: "B/Other.song", Title: "Other", Artist: "B", Lyrics: "a word about sunshine late in the song"},
	}
	for _, r := range ctxSeed {
		r.Slug = SlugForPath(r.RelPath)
		r.Hash = strings.Repeat("b", 64)
		if _, err := x.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	hits, _, err := x.List(ctx, Query{Text: "sunshine"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].Title != "Sunshine" {
		t.Errorf("ranked %v first, want the title match", titles(hits))
	}
}

// The excerpt must arrive as fragments with the match flagged, never as markup.
// This is what keeps a lyric containing a script tag from becoming one.
func TestSearchSnippetsAreFragmentsNotMarkup(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	r := Record{
		RelPath: "A/Evil.song", Slug: "a-evil", Title: "Evil", Artist: "A",
		Hash:   strings.Repeat("c", 64),
		Lyrics: `a line with <script>alert(1)</script> and the word wretch in it`,
	}
	if _, err := x.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	hits, _, err := x.List(ctx, Query{Text: "wretch"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits", len(hits))
	}
	frags := hits[0].Frags
	if len(frags) < 2 {
		t.Fatalf("expected the excerpt to be split into fragments, got %v", frags)
	}

	var hit, all string
	for _, f := range frags {
		all += f.Text
		if f.Hit {
			hit += f.Text
		}
		// No fragment may carry a sentinel: they are the split markers and must
		// be consumed, or they would reach the page as control characters.
		if strings.ContainsAny(f.Text, hitOpen+hitClose) {
			t.Errorf("fragment %q still holds a marker", f.Text)
		}
	}
	if !strings.Contains(strings.ToLower(hit), "wretch") {
		t.Errorf("the flagged fragment is %q, want the matched word", hit)
	}
	// The script tag survives as ordinary text, to be escaped by the template.
	if !strings.Contains(all, "<script>") {
		t.Errorf("excerpt lost the lyric text: %q", all)
	}
	if strings.Contains(all, "<mark>") {
		t.Error("the database produced markup; <mark> belongs in the template")
	}
}

// Hostile search input must return results or none, never an error. An
// unescaped quote or a stray NEAR( reaching the full-text parser is the most
// common way this kind of application produces a 500.
func TestSearchNeverFailsOnHostileInput(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	hostile := []string{
		`' OR 1=1 --`,
		`"; DROP TABLE songs; --`,
		`"`,
		`""`,
		`"""`,
		`*`,
		`^`,
		`-`,
		`NEAR(a b, 2)`,
		`a OR b`,
		`a AND`,
		`AND`,
		`NOT`,
		`(((`,
		`)))`,
		`a:b:c`,
		`tag:`,
		`chord:`,
		`{}`,
		`[]`,
		`a*`,
		`**`,
		`\`,
		`%`,
		`_`,
		"\x00",
		"\x02\x03",
		strings.Repeat(`"`, 500),
		strings.Repeat("a ", 1000),
		strings.Repeat("é", 500),
		"amazing OR (grace AND NEAR(x))",
		"1' UNION SELECT value FROM settings --",
	}
	for _, text := range hostile {
		name := text
		if len(name) > 24 {
			name = name[:24] + "..."
		}
		t.Run(name, func(t *testing.T) {
			if _, _, err := x.List(ctx, Query{Text: text}); err != nil {
				t.Errorf("List(%q) returned an error: %v", text, err)
			}
			// And through the front door, where the user's raw string arrives.
			if _, _, err := x.List(ctx, ParseQuery(text)); err != nil {
				t.Errorf("List(ParseQuery(%q)) returned an error: %v", text, err)
			}
		})
	}
	// The database must be intact afterwards.
	if s, _ := x.Stats(ctx); s.Songs != 3 {
		t.Fatalf("the collection now holds %d songs, want 3", s.Songs)
	}
	if v, err := x.Setting(ctx, settingDerivedVersion); err != nil || v == "" {
		t.Errorf("settings table damaged: %q %v", v, err)
	}
}

func TestFilters(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"tag", Query{Tag: "hymn"}, []string{"Amazing Grace"}},
		{"tag is accent and case insensitive", Query{Tag: "Canción"}, []string{"Y nos dieron las diez"}},
		{"artist", Query{Artist: "Bill Withers"}, []string{"Ain't No Sunshine"}},
		{"key", Query{Key: "Am"}, []string{"Ain't No Sunshine", "Y nos dieron las diez"}},
		{"chord", Query{Chord: "Em"}, []string{"Ain't No Sunshine", "Amazing Grace"}},
		{"favorite", Query{Favorite: true}, []string{"Y nos dieron las diez"}},
		{"unknown tag", Query{Tag: "nonexistent"}, nil},
		{"text and filter together", Query{Text: "am", Tag: "soul"}, []string{"Ain't No Sunshine"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.q
			if q.Sort == "" {
				q.Sort = SortTitle
			}
			hits, total, err := x.List(ctx, q)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if total != len(tt.want) {
				t.Fatalf("got %d hits %v, want %d %v", total, titles(hits), len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if hits[i].Title != w {
					t.Errorf("hit %d = %q, want %q (all: %v)", i, hits[i].Title, w, titles(hits))
				}
			}
		})
	}
}

func TestSortOrders(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	// Give one song a recent view so SortRecent has something to order by.
	if err := x.RecordView(ctx, SlugForPath("Bill Withers/Ain't No Sunshine.song"), timeNow()); err != nil {
		t.Fatal(err)
	}

	byTitle, _, err := x.List(ctx, Query{Sort: SortTitle})
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(byTitle); got[0] != "Ain't No Sunshine" || got[len(got)-1] != "Y nos dieron las diez" {
		t.Errorf("title order = %v", got)
	}

	byArtist, _, err := x.List(ctx, Query{Sort: SortArtist})
	if err != nil {
		t.Fatal(err)
	}
	// Bill Withers, Joaquín Sabina, Traditional.
	if got := titles(byArtist); got[0] != "Ain't No Sunshine" {
		t.Errorf("artist order = %v", got)
	}

	byRecent, _, err := x.List(ctx, Query{Sort: SortRecent})
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(byRecent); got[0] != "Ain't No Sunshine" {
		t.Errorf("recent order = %v, want the viewed song first", got)
	}
	// Songs with no view at all must still appear, just afterwards.
	if len(byRecent) != 3 {
		t.Errorf("recent order dropped songs that were never opened: %v", titles(byRecent))
	}

	if _, _, err := x.List(ctx, Query{Sort: SortAdded}); err != nil {
		t.Fatal(err)
	}
}

func TestListPaginates(t *testing.T) {
	ctx := context.Background()
	x, _ := newIndex(t)
	seed(t, x)

	page1, total, err := x.List(ctx, Query{Sort: SortTitle, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want the full count regardless of the page size", total)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 has %d hits, want 2", len(page1))
	}
	page2, _, err := x.List(ctx, Query{Sort: SortTitle, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Errorf("page 2 has %d hits, want 1", len(page2))
	}
	if page1[0].Title == page2[0].Title {
		t.Error("the pages overlap")
	}
	// A silly limit is clamped rather than honoured.
	if hits, _, _ := x.List(ctx, Query{Limit: 100000}); len(hits) > 200 {
		t.Errorf("limit was not clamped: %d hits", len(hits))
	}
}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		raw  string
		want Query
	}{
		{"amazing grace", Query{Text: "amazing grace", Sort: SortRelevance}},
		{"tag:hymn", Query{Tag: "hymn", Sort: SortTitle}},
		{"tag:hymn amazing", Query{Text: "amazing", Tag: "hymn", Sort: SortRelevance}},
		{"artist:Sabina", Query{Artist: "Sabina", Sort: SortTitle}},
		{"chord:F#m", Query{Chord: "F#m", Sort: SortTitle}},
		{"key:Am", Query{Key: "Am", Sort: SortTitle}},
		// Spanish field names, since the interface is in Spanish.
		{"etiqueta:himno", Query{Tag: "himno", Sort: SortTitle}},
		{"artista:Sabina", Query{Artist: "Sabina", Sort: SortTitle}},
		{"acorde:Am", Query{Chord: "Am", Sort: SortTitle}},
		{"tono:G", Query{Key: "G", Sort: SortTitle}},
		// An unknown prefix is not a field, so it stays as search text.
		{"nonsense:value", Query{Text: "nonsense:value", Sort: SortRelevance}},
		// A prefix with no value is text too, not an empty filter.
		{"tag:", Query{Text: "tag:", Sort: SortRelevance}},
		{"", Query{Sort: SortTitle}},
	}
	for _, tt := range tests {
		got := ParseQuery(tt.raw)
		if got != tt.want {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.raw, got, tt.want)
		}
	}
}

func TestFTSQuery(t *testing.T) {
	tests := []struct{ in, want string }{
		{"amazing", `"amazing"*`},
		{"amazing grace", `"amazing"* AND "grace"*`},
		{"", ""},
		{"   ", ""},
		{`"`, ""}, // punctuation only: nothing to search for
		{`NEAR(a, 2)`, `"NEAR"* AND "a"* AND "2"*`},
		{"ain't", `"ain"* AND "t"*`},
		{"a-b", `"a"* AND "b"*`},
		{"corazón", `"corazón"*`},
	}
	for _, tt := range tests {
		if got := ftsQuery(tt.in); got != tt.want {
			t.Errorf("ftsQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// Long input is bounded rather than turned into a huge expression.
	if got := ftsQuery(strings.Repeat("word ", 100)); strings.Count(got, " AND ") != maxTerms-1 {
		t.Errorf("term count was not capped: %d terms", strings.Count(got, " AND ")+1)
	}
	if got := ftsQuery(strings.Repeat("a", 500)); len(got) > maxTermRunes+8 {
		t.Errorf("term length was not capped: %q", got)
	}
}

// No input may produce an expression that FTS5 rejects.
func FuzzFTSQuery(f *testing.F) {
	x, _ := newIndexForFuzz(f)
	ctx := context.Background()
	for _, s := range []string{
		"amazing", `"`, `NEAR(a b, 2)`, `' OR 1=1 --`, "*", "^", "corazón",
		strings.Repeat(`"`, 50), "a OR b", "", "\x00", "tag:hymn",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if _, _, err := x.List(ctx, Query{Text: in}); err != nil {
			t.Fatalf("List(%q) failed: %v", in, err)
		}
		if _, _, err := x.List(ctx, ParseQuery(in)); err != nil {
			t.Fatalf("List(ParseQuery(%q)) failed: %v", in, err)
		}
	})
}
