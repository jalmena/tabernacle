package web

import (
	"net/http"
	"strconv"

	"github.com/jalmena/tabernacle/internal/index"
)

const perPage = 40

// libraryBody is what the library template receives.
type libraryBody struct {
	Result   library_result
	Tags     []index.TagCount
	Raw      string // the search box's current contents
	Filters  []filterChip
	Sort     string
	SortURLs []sortOption
	PrevURL  string
	NextURL  string
	Empty    emptyState
}

// library_result mirrors what Browse returned, flattened for the template.
type library_result struct {
	Hits   []songRow
	Total  int
	Offset int
	Shown  int
}

// songRow is one line of the library list.
type songRow struct {
	Title    string
	Artist   string
	URL      string
	Key      string
	Capo     int
	Tags     []string
	Chords   []string
	Favorite bool
	Broken   bool
	Frags    []index.Frag
}

// filterChip is an active filter, with a URL that removes it.
type filterChip struct {
	Label    string
	ClearURL string
}

type sortOption struct {
	Label string
	URL   string
	On    bool
}

// emptyState explains an empty list, which differs a lot between "you have no
// songs yet" and "that search found nothing".
type emptyState struct {
	Searching bool
	SongsDir  string
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	raw := qs.Get("q")

	q := index.ParseQuery(raw)
	// Explicit parameters win over anything inferred from the search text, so a
	// tag link is not overridden by whatever is in the box.
	if v := qs.Get("tag"); v != "" {
		q.Tag = v
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
	if qs.Get("fav") == "1" {
		q.Favorite = true
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

	res, err := s.lib.Browse(r.Context(), q)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	tags, err := s.lib.Tags(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	body := libraryBody{
		Raw:  raw,
		Sort: string(q.Sort),
		Result: library_result{
			Total:  res.Total,
			Offset: q.Offset,
			Shown:  len(res.Hits),
		},
		Tags: tags,
		Empty: emptyState{
			Searching: raw != "" || q.Tag != "" || q.Artist != "" || q.Chord != "" ||
				q.Key != "" || q.Favorite || q.Broken,
			SongsDir: s.cfg.SongsDir,
		},
	}

	for _, h := range res.Hits {
		body.Result.Hits = append(body.Result.Hits, songRow{
			Title:    h.Title,
			Artist:   h.Artist,
			URL:      "/songs/" + h.Slug,
			Key:      h.Key,
			Capo:     h.Capo,
			Tags:     h.Tags,
			Chords:   h.Chords,
			Favorite: h.Favorite(),
			Broken:   h.Broken(),
			Frags:    h.Frags,
		})
	}

	// Active filters, each with a link that drops just that one.
	if q.Tag != "" {
		body.Filters = append(body.Filters, filterChip{
			Label: "etiqueta: " + q.Tag, ClearURL: libraryURL(qs, "tag", ""),
		})
	}
	if q.Artist != "" {
		body.Filters = append(body.Filters, filterChip{
			Label: "artista: " + q.Artist, ClearURL: libraryURL(qs, "artist", ""),
		})
	}
	if q.Chord != "" {
		body.Filters = append(body.Filters, filterChip{
			Label: "acorde: " + q.Chord, ClearURL: libraryURL(qs, "chord", ""),
		})
	}
	if q.Key != "" {
		body.Filters = append(body.Filters, filterChip{
			Label: "tono: " + q.Key, ClearURL: libraryURL(qs, "key", ""),
		})
	}
	if q.Favorite {
		body.Filters = append(body.Filters, filterChip{
			Label: "favoritas", ClearURL: libraryURL(qs, "fav", ""),
		})
	}
	if q.Broken {
		body.Filters = append(body.Filters, filterChip{
			Label: "con errores", ClearURL: libraryURL(qs, "broken", ""),
		})
	}

	for _, opt := range []struct {
		label string
		order index.SortOrder
	}{
		{"Título", index.SortTitle},
		{"Artista", index.SortArtist},
		{"Recientes", index.SortRecent},
		{"Añadidas", index.SortAdded},
	} {
		body.SortURLs = append(body.SortURLs, sortOption{
			Label: opt.label,
			URL:   libraryURL(qs, "sort", string(opt.order)),
			On:    q.Sort == opt.order,
		})
	}

	if page > 1 {
		body.PrevURL = libraryURL(qs, "page", strconv.Itoa(page-1))
	}
	if res.HasMore() {
		body.NextURL = libraryURL(qs, "page", strconv.Itoa(page+1))
	}

	s.render(w, r, http.StatusOK, "library.html", page_(title(res.Total, raw), raw, body))
}

// libraryURL rewrites one parameter of the current query, dropping it when the
// value is empty and always resetting pagination, since page three of a
// different filter is meaningless.
func libraryURL(qs map[string][]string, key, value string) string {
	next := make(map[string][]string, len(qs)+1)
	for k, v := range qs {
		if k == key || k == "page" {
			continue
		}
		next[k] = v
	}
	if value != "" {
		next[key] = []string{value}
	}
	if key != "page" {
		delete(next, "page")
	}
	return "/" + encodeQuery(next)
}

func title(total int, raw string) string {
	if raw != "" {
		return "Buscar: " + raw
	}
	return "Tabernacle"
}

// page_ builds the template shell. The trailing underscore keeps it from
// colliding with the page type.
func page_(t, query string, body any) page {
	return page{Title: t, Nav: navState{Query: query}, Body: body}
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.lib.Tags(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	artists, err := s.lib.Artists(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "tags.html", page_("Etiquetas", "", struct {
		Tags    []index.TagCount
		Artists []index.ArtistCount
	}{tags, artists}))
}
