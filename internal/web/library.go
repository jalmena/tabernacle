package web

import (
	"net/http"
	"strconv"

	"github.com/jalmena/tabernacle/internal/index"
)

const perPage = 40

// libraryBody is what the song list receives.
type libraryBody struct {
	Total   int
	Offset  int
	Hits    []songRow
	Raw     string // the search box's current contents
	Filters []filterChip
	PrevURL string
	NextURL string
	Empty   emptyState
}

// songRow is one line of the song list.
type songRow struct {
	Title  string
	Artist string
	Genre  string
	URL    string
	Key    string
	Capo   int
	Broken bool
	Frags  []index.Frag
}

// filterChip is an active filter, with a URL that removes it.
type filterChip struct {
	Label    string
	ClearURL string
}

// emptyState explains an empty list, which reads very differently between "you
// have no songs yet" and "that search found nothing".
type emptyState struct {
	Searching bool
	SongsDir  string
}

// queryFrom builds a listing query from the request.
func queryFrom(r *http.Request) (index.Query, int, string) {
	qs := r.URL.Query()
	raw := qs.Get("q")

	q := index.ParseQuery(raw)
	// An explicit parameter wins over anything inferred from the search text, so
	// a genre link is not overridden by whatever is in the box.
	if v := qs.Get("genre"); v != "" {
		q.Genre = v
	}
	if v := qs.Get("artist"); v != "" {
		q.Artist = v
	}
	if v := qs.Get("chord"); v != "" {
		q.Chord = v
	}
	if v := qs.Get("key"); v != "" {
		q.Key = v
	}
	if qs.Get("broken") == "1" {
		q.Broken = true
	}
	if v := qs.Get("sort"); v != "" {
		switch index.SortOrder(v) {
		case index.SortTitle, index.SortArtist, index.SortRecent, index.SortAdded, index.SortRelevance:
			q.Sort = index.SortOrder(v)
		}
	}

	page := 1
	if n, err := strconv.Atoi(qs.Get("page")); err == nil && n > 1 {
		page = n
	}
	q.Limit = perPage
	q.Offset = (page - 1) * perPage
	return q, page, raw
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	q, page, raw := queryFrom(r)

	res, err := s.lib.Browse(r.Context(), q)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	body := libraryBody{
		Total:  res.Total,
		Offset: q.Offset,
		Raw:    raw,
		Empty: emptyState{
			Searching: raw != "" || q.Genre != "" || q.Artist != "" ||
				q.Chord != "" || q.Key != "" || q.Broken,
			SongsDir: s.cfg.SongsDir,
		},
	}
	for _, h := range res.Hits {
		body.Hits = append(body.Hits, songRow{
			Title:  h.Title,
			Artist: h.Artist,
			Genre:  h.Genre,
			URL:    "/songs/" + h.Slug,
			Key:    h.Key,
			Capo:   h.Capo,
			Broken: h.Broken(),
			Frags:  h.Frags,
		})
	}

	qs := r.URL.Query()
	for _, f := range []struct{ key, label, value string }{
		{"genre", "género", q.Genre},
		{"artist", "artista", q.Artist},
		{"chord", "acorde", q.Chord},
		{"key", "tono", q.Key},
	} {
		if f.value != "" {
			body.Filters = append(body.Filters, filterChip{
				Label:    f.label + ": " + f.value,
				ClearURL: libraryURL(qs, f.key, ""),
			})
		}
	}
	if q.Broken {
		body.Filters = append(body.Filters, filterChip{
			Label: "con errores", ClearURL: libraryURL(qs, "broken", ""),
		})
	}

	if page > 1 {
		body.PrevURL = libraryURL(qs, "page", strconv.Itoa(page-1))
	}
	if res.HasMore() {
		body.NextURL = libraryURL(qs, "page", strconv.Itoa(page+1))
	}

	title := "Canciones"
	if raw != "" {
		title = "Buscar: " + raw
	}
	s.render(w, r, http.StatusOK, "library.html", s.page(r, title, body))
}

// handleArtists lists the artists in the collection.
func (s *Server) handleArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := s.lib.Artists(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "artists.html", s.page(r, "Artistas", artists))
}

// handleGenres lists the genres in the collection.
func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := s.lib.Genres(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "genres.html", s.page(r, "Géneros", genres))
}

// libraryURL rewrites one parameter of the current query.
//
// The parameter is dropped when the value is empty, pagination is always reset
// because page three of a different filter is meaningless, and the flash code is
// dropped because a message is meant to be seen once — carrying it into every
// filter link would make it reappear on the next click.
func libraryURL(qs map[string][]string, key, value string) string {
	next := make(map[string][]string, len(qs)+1)
	for k, v := range qs {
		if k == key || k == "page" || k == "msg" {
			continue
		}
		next[k] = v
	}
	if value != "" {
		next[key] = []string{value}
	}
	return "/" + encodeQuery(next)
}
