package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/render"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// maxSymbolBytes caps the chord symbol taken from a query string.
//
// The longest thing this can legitimately be is a slash chord with a long
// suffix; anything past that is somebody probing, and a symbol that fails to
// parse is echoed back inside the page, so it does not get to be long.
const maxSymbolBytes = 32

// handleChords is the chord page: draw a shape and be told what it is, or name a
// chord and be shown every way to hold it.
//
// All of the state is in the query, so any view of it can be linked to, put in a
// bookmark, and reached again with the back button.
func (s *Server) handleChords(w http.ResponseWriter, r *http.Request) {
	v := s.labFor(r)
	s.render(w, r, http.StatusOK, "chords.html", s.page(r, "Acordes", v))
}

// handleChordFragment returns the part of the page that depends on the shape, so
// that editing the neck does not reload the neck. Without JavaScript nothing
// asks for it and the form reloads the page instead.
func (s *Server) handleChordFragment(w http.ResponseWriter, r *http.Request) {
	v := s.labFor(r)
	s.render(w, r, http.StatusOK, "chordfragment.html", s.page(r, "Acordes", v))
}

// labFor reads the query.
//
// Nothing here can fail: a symbol nobody can parse, a fret nobody has and a
// window off the end of the neck all produce a page with an honest note on it,
// never a 400. A reference that answers a fumbled URL with an error page is
// telling somebody they typed wrong when it could simply have shown them the
// chord page.
func (s *Server) labFor(r *http.Request) render.LabView {
	q := r.URL.Query()

	sym := strings.TrimSpace(q.Get("c"))
	if len(sym) > maxSymbolBytes {
		sym = ""
	}
	grip := gripFromQuery(q.Get("grip"), q)

	// Zero means "no window asked for", which lets the page open on whatever
	// window shows the chord it was given.
	start, _ := strconv.Atoi(q.Get("start"))

	return render.Lab(render.LabOptions{
		Notation: s.notationFor(r, ""),
		Lang:     s.langFor(r),
		Start:    start,
		Grip:     grip,
		Symbol:   sym,
		All:      q.Get("all") == "1",
	})
}

// gripFromQuery reads the shape drawn on the neck.
//
// The form sends one value per string, because a group of radio buttons is one
// choice and there are six strings; the compact "x-3-2-0-1-0" is what a link
// carries. Both arrive here and mean the same thing, with the form winning when
// it is complete, because it is the more recent of the two.
func gripFromQuery(compact string, q url.Values) string {
	n := voicing.Standard().Strings()
	parts := make([]string, 0, n)
	for i := range n {
		v := q.Get("s" + strconv.Itoa(i))
		if v == "" {
			break
		}
		parts = append(parts, v)
	}
	if len(parts) == n {
		if grip := strings.Join(parts, "-"); gripOK(grip) {
			return grip
		}
	}
	if gripOK(compact) {
		return compact
	}
	return ""
}

func gripOK(s string) bool {
	frets, ok := voicing.ParseGrip(s)
	return ok && len(frets) == voicing.Standard().Strings()
}
