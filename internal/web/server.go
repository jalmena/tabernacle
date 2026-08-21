// Package web serves Tabernacle over HTTP.
//
// Templates and assets are embedded, so the binary is self-contained and the
// application works with no network access of any kind once installed.
//
// Two rules run through this package. Every value handed to a template is a
// plain string, so html/template escapes it in the right context and nothing
// here builds markup. And every URL is built in Go rather than assembled in a
// template, so escaping a query parameter cannot be forgotten.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/render"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server holds everything the handlers need.
type Server struct {
	lib *library.Library
	cfg config.Config
	log *slog.Logger

	tpl *template.Template
	// assetVersion changes whenever an embedded asset changes, which is what
	// lets the stylesheet be cached forever and still update on a new release.
	assetVersion string
}

// New parses the templates and prepares the server.
func New(lib *library.Library, cfg config.Config, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	tpl, err := template.New("").Funcs(funcs()).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	version, err := hashAssets()
	if err != nil {
		return nil, err
	}
	return &Server{lib: lib, cfg: cfg, log: log, tpl: tpl, assetVersion: version}, nil
}

// funcs holds the template helpers.
//
// All but one are value transforms that return plain strings, which is what
// keeps the escaping in html/template's hands. The exception, chordDiagram, is
// documented where it is defined.
func funcs() template.FuncMap {
	return template.FuncMap{
		"diagram": chordDiagram,
		"signed": func(n int) string {
			if n > 0 {
				return "+" + strconv.Itoa(n)
			}
			return strconv.Itoa(n)
		},
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
	}
}

// hashAssets fingerprints the embedded assets so their URLs change when they do.
func hashAssets() (string, error) {
	sum := sha256.New()
	var names []string
	err := fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, p)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("web: read assets: %w", err)
	}
	sort.Strings(names) // walk order is stable, but do not rely on it
	for _, n := range names {
		b, err := staticFS.ReadFile(n)
		if err != nil {
			return "", err
		}
		sum.Write([]byte(n))
		sum.Write(b)
	}
	return hex.EncodeToString(sum.Sum(nil))[:12], nil
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleLibrary)
	mux.HandleFunc("GET /songs/{slug}", s.handleSong)
	mux.HandleFunc("GET /songs/{slug}/sheet", s.handleSheetFragment)
	mux.HandleFunc("GET /songs/{slug}/print", s.handlePrint)
	mux.HandleFunc("GET /songs/{slug}/raw", s.handleRaw)
	mux.HandleFunc("GET /songs/new", s.handleNewSong)
	mux.HandleFunc("GET /songs/{slug}/edit", s.handleEditSong)
	mux.HandleFunc("GET /songs/{slug}/delete", s.handleDeleteForm)
	mux.HandleFunc("GET /artists", s.handleArtists)
	mux.HandleFunc("GET /genres", s.handleGenres)
	mux.HandleFunc("GET /import", s.handleImportForm)
	mux.HandleFunc("GET /export", s.handleExport)

	// Everything that changes the collection goes through s.mutating, which
	// applies the read-only guard, the same-origin check and the body limit in
	// one place, so a new write route cannot be added without them.
	mux.HandleFunc("POST /songs/new", s.mutatingMultipart(s.handleLoadIntoEditor))
	mux.HandleFunc("POST /songs", s.mutating(s.handleCreateSong))
	mux.HandleFunc("POST /songs/{slug}", s.mutating(s.handleSaveSong))
	mux.HandleFunc("POST /songs/{slug}/delete", s.mutating(s.handleDelete))
	mux.HandleFunc("POST /songs/{slug}/transpose", s.mutating(s.handleRememberTranspose))
	mux.HandleFunc("POST /songs/{slug}/commit-transpose", s.mutating(s.handleCommitTranspose))
	mux.HandleFunc("POST /reindex", s.mutating(s.handleReindex))
	// Import goes through the same guard, but reads its own body because it is
	// multipart rather than a form.
	mux.HandleFunc("POST /import", s.mutatingMultipart(s.handleImport))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /static/{version}/{path...}", s.handleStatic)

	// A catch-all for anything unmatched, so a mistyped address gets Tabernacle's
	// own page with the navigation on it rather than a bare line of text with no
	// way back. Registered for GET only, which leaves the wrong method on a real
	// route answering 405 as it should.
	mux.HandleFunc("GET /", s.handleNotFound)

	// Outermost first. The host check comes before anything that reads the
	// request, and the panic guard wraps everything so a bug is a 500 rather
	// than a dead connection.
	var h http.Handler = mux
	h = s.securityHeaders(h)
	h = s.requireAllowedHost(h)
	h = s.logRequests(h)
	h = s.recoverPanics(h)
	return h
}

// page is the common shell every template receives.
type page struct {
	Title        string
	AssetVersion string
	Nav          navState
	Body         any
}

// navState is what the header needs to render itself.
//
// It is built for every page, not just the listing, because navigation that
// disappears on the import or editor page leaves the reader stranded.
type navState struct {
	Query    string
	ReadOnly bool
	Flash    string
	Section  string    // which of the sections is current
	Sections []navLink // Canciones, Artistas, Géneros, Añadir
}

// navLink is one entry in the header.
type navLink struct {
	Label string
	URL   string
	On    bool
}

// page builds the shell every template receives, including the navigation.
//
// Every page gets the same row of sections, so nothing has to vanish just
// because the reader is on the import or editor page.
//
// There is no visible ordering control. The ?sort= parameter still works for a
// link, but a row of orderings beside a row of sections was two vocabularies
// competing for the same glance, and the collection is alphabetical by default —
// which is what somebody looking for a song by name actually wants.
func (s *Server) page(r *http.Request, title string, body any) page {
	qs := r.URL.Query()
	path := r.URL.Path

	section := ""
	switch {
	case path == "/songs/new" || path == "/import":
		section = "add"
	case path == "/" || strings.HasPrefix(path, "/songs"):
		section = "songs"
	case path == "/artists":
		section = "artists"
	case path == "/genres":
		section = "genres"
	}

	// Four sections and nothing else. Anything more and two labels start to look
	// like each other, which is how "Artista" the order came to sit next to
	// "Artistas" the section.
	nav := navState{
		Query:   qs.Get("q"),
		Section: section,
		Sections: []navLink{
			{Label: "Canciones", URL: "/", On: section == "songs"},
			{Label: "Artistas", URL: "/artists", On: section == "artists"},
			{Label: "Géneros", URL: "/genres", On: section == "genres"},
		},
	}
	if !s.cfg.ReadOnly {
		nav.Sections = append(nav.Sections,
			navLink{Label: "Añadir", URL: "/songs/new", On: section == "add"})
	}
	return page{Title: title, Nav: nav, Body: body}
}

// render executes a template into a buffer before writing anything.
//
// Buffering matters: a template that fails halfway through would otherwise
// leave a truncated page already committed with a 200.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, p page) {
	p.AssetVersion = s.assetVersion
	p.Nav.ReadOnly = s.cfg.ReadOnly
	if p.Nav.Flash == "" {
		p.Nav.Flash = flashFor(r)
	}

	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, p); err != nil {
		s.log.Error("template failed", "template", name, "path", r.URL.Path, "error", err)
		http.Error(w, "Error interno del servidor", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// fail shows an error page. The message is chosen by the caller from a fixed
// set; nothing from the request is ever echoed back.
//
// It goes through s.page like everything else, so an error page carries the
// navigation too. A 404 that strands the reader with no way out is a worse
// failure than whatever they mistyped.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, heading, detail string) {
	s.render(w, r, status, "error.html",
		s.page(r, heading, errorBody{Status: status, Heading: heading, Detail: detail}))
}

type errorBody struct {
	Status  int
	Heading string
	Detail  string
}

// serverError logs the real cause and shows the user something useful but
// uninformative, so an internal detail never leaks into a page.
func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	s.fail(w, r, http.StatusInternalServerError, "Algo ha ido mal",
		"No se ha podido completar la operación. Revisa el registro del servidor para más detalles.")
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.fail(w, r, http.StatusNotFound, "Aquí no hay nada",
		"Esa dirección no existe. Prueba a buscar la canción desde la biblioteca.")
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// A personal songbook has no business in a search index, and a crawler that
	// found one would be reaching something it should not.
	_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	// Reject anything that is not a plain forward-slash relative name. The
	// embedded filesystem would refuse an escape anyway; this keeps the reason
	// explicit.
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if r.PathValue("version") == s.assetVersion {
		// The URL carries the content hash, so this response can never go stale.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	stats, err := s.lib.Stats(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"unavailable"}`)
		return
	}
	fmt.Fprintf(w, `{"status":"ok","songs":%d,"artists":%d,"broken":%d}`,
		stats.Songs, stats.Artists, stats.Broken)
}

// notationFor decides which note names to show.
//
// Precedence is deliberate: an explicit query parameter wins so a link can carry
// a notation and a printed page is reproducible, then the reader's stored
// preference, then whatever the deployment configured.
func (s *Server) notationFor(r *http.Request, stored string) chord.Notation {
	pick := r.URL.Query().Get("n")
	if pick == "" {
		pick = stored
	}
	if pick == "" {
		pick = s.cfg.Notation
	}
	if pick == config.NotationSolfege {
		return chord.Solfege
	}
	return chord.English
}

// transposeFor reads the requested transposition, falling back to whatever the
// reader saved for this song.
//
// A value that is not a number is ignored rather than rejected: a mangled link
// should show the song, not an error page.
func transposeFor(r *http.Request, stored int) int {
	raw := r.URL.Query().Get("t")
	if raw == "" {
		return render.NormalizeTranspose(stored)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return render.NormalizeTranspose(n)
}

func capoViewFor(r *http.Request, stored string) render.CapoView {
	pick := r.URL.Query().Get("capo")
	if pick == "" {
		pick = stored
	}
	if pick == string(render.CapoSounding) {
		return render.CapoSounding
	}
	return render.CapoWritten
}
