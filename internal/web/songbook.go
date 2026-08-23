package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/render"
)

// How many songs one book may hold.
//
// The explicit form carries a slug per song, so a cap keeps the URL inside every
// real limit: sixty slugs is about 1.7 kB. The filter form names no songs at all,
// so it can afford a larger one.
const (
	maxSongbookSongs = 60
	maxSongbookAll   = 200
)

// errTooManySongs refuses a book nobody asked to be that long.
var errTooManySongs = errors.New("web: too many songs for one book")

// songbookBody is the page where a book is chosen.
type songbookBody struct {
	Tabs     []navLink
	Rows     []songbookRow
	Total    int
	Raw      string
	Papers   []themeChoice
	Solfege  bool
	Filtered bool
}

type songbookRow struct {
	Slug   string
	Title  string
	Artist string
	Key    string
}

// songbookPrintBody is the book itself.
type songbookPrintBody struct {
	Theme  string
	Index  bool
	Songs  []songbookSheet
	BackTo string
	// Skipped counts songs that were asked for and are no longer there, so a
	// stale bookmark prints what it still can and says what it could not.
	Skipped int
}

type songbookSheet struct {
	Sheet   render.SheetView
	OrigKey string
	Step    string
	// TwoColumns is set for a song too long to fit one side of paper. Two
	// columns hold about twice as much, which is the difference between a song
	// you can see whole and one you have to turn over mid-verse.
	TwoColumns bool
}

// linesPerPage is how many lines of type one side of A4 holds at the printed
// size, once the song's own heading is out of the way.
//
// It decides one thing: whether a song is set in one column or two. A little
// generous on purpose — a song that spills by two lines is better turned over
// than squeezed into columns it does not need.
const linesPerPage = 46

// handleSongbook offers the collection with a checkbox on each row.
//
// Public, like every other way of reading. It is merely linked from the settings
// page, because that is where somebody looking for it will go.
func (s *Server) handleSongbook(w http.ResponseWriter, r *http.Request) {
	q, _, raw := queryFrom(r)
	q.Limit, q.Offset = maxSongbookAll, 0

	res, err := s.lib.Browse(r.Context(), q)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	body := songbookBody{
		Tabs:     s.settingsTabs(r, "songbook"),
		Total:    res.Total,
		Raw:      raw,
		Papers:   paperChoices(printThemeFor(r)),
		Solfege:  r.URL.Query().Get("n") == "solfege",
		Filtered: q.Text != "" || q.Genre != "" || q.Artist != "" || q.Chord != "" || q.Key != "",
	}
	for _, h := range res.Hits {
		body.Rows = append(body.Rows, songbookRow{
			Slug: h.Slug, Title: h.Title, Artist: h.Artist, Key: h.Key,
		})
	}
	s.render(w, r, http.StatusOK, "songbook.html", s.page(r, "Generar cancionero", body))
}

// handleSongbookPrint renders many songs as one printable document.
func (s *Server) handleSongbookPrint(w http.ResponseWriter, r *http.Request) {
	slugs, err := s.songbookSlugs(r)
	switch {
	case errors.Is(err, errTooManySongs):
		s.fail(w, r, http.StatusBadRequest, "Demasiadas canciones",
			"Un cancionero admite hasta 60 canciones elegidas una a una. Para más, "+
				"filtra la biblioteca y marca «todas las de esta búsqueda».")
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	case len(slugs) == 0:
		s.redirect(w, r, songbookBackURL(r), "none-picked")
		return
	}

	notation := chord.English
	if r.URL.Query().Get("n") == "solfege" {
		notation = chord.Solfege
	}

	body := songbookPrintBody{
		Theme:  printThemeFor(r),
		Index:  r.URL.Query().Get("index") == "1",
		BackTo: songbookBackURL(r),
	}
	for _, slug := range slugs {
		view, err := s.lib.Get(r.Context(), slug)
		if err != nil {
			// Renamed or deleted since the link was made. Printing the rest is
			// more use than refusing the lot.
			body.Skipped++
			continue
		}
		// Each song keeps the key its reader asked to remember, which is what
		// "Recordar el tono" is for. Diagrams are off: sixty chord appendices are
		// not a songbook.
		sheet := render.Sheet(view.Song, render.Options{
			Transpose: view.State.Transpose,
			Notation:  notation,
			Lang:      s.langFor(r),
			Print:     true,
		})
		body.Songs = append(body.Songs, songbookSheet{
			Sheet:      sheet,
			OrigKey:    sheet.OrigKey,
			Step:       stepShort(view.State.Transpose),
			TwoColumns: sheet.PrintedLines() > linesPerPage,
		})
	}
	if len(body.Songs) == 0 {
		s.fail(w, r, http.StatusNotFound, "Ninguna canción",
			"Ninguna de las canciones elegidas sigue en la colección.")
		return
	}
	s.render(w, r, http.StatusOK, "songbookprint.html", s.page(r, "Cancionero", body))
}

// songbookSlugs resolves which songs a book holds, from either form the print
// route accepts.
//
// Ticking boxes produces a slug per song. The filter form exists because "print
// every bolero" would otherwise be a URL with forty slugs in it, and because the
// explicit form has to have a limit somewhere.
func (s *Server) songbookSlugs(r *http.Request) ([]string, error) {
	if r.URL.Query().Get("all") == "1" {
		q, _, _ := queryFrom(r)
		q.Limit, q.Offset = maxSongbookAll, 0
		res, err := s.lib.Browse(r.Context(), q)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(res.Hits))
		for _, h := range res.Hits {
			out = append(out, h.Slug)
		}
		return out, nil
	}

	// Duplicates dropped, order kept: the listing's order is the book's order,
	// which is what the sort parameter is for.
	seen := make(map[string]bool)
	out := make([]string, 0, len(r.URL.Query()["s"]))
	for _, slug := range r.URL.Query()["s"] {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	if len(out) > maxSongbookSongs {
		return nil, errTooManySongs
	}
	return out, nil
}

// paperChoices lists the paper options as radio values rather than links, because
// on these two pages the paper travels with a form.
func paperChoices(current string) []themeChoice {
	out := make([]themeChoice, 0, len(printThemeList))
	for _, t := range printThemeList {
		out = append(out, themeChoice{ID: t.id, Label: t.label, On: t.id == current})
	}
	return out
}

// songbookBackURL returns to the chooser with the same search still applied.
func songbookBackURL(r *http.Request) string {
	qs := url.Values{}
	for _, k := range []string{"q", "genre", "artist", "chord", "key", "sort", "n"} {
		if v := r.URL.Query().Get(k); v != "" {
			qs.Set(k, v)
		}
	}
	if len(qs) == 0 {
		return "/songbook"
	}
	return "/songbook?" + qs.Encode()
}
