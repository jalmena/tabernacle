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

	"github.com/jalmena/tabernacle/internal/account"
	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/i18n"
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
	acc *account.Store
	cfg config.Config
	log *slog.Logger

	// logins refuses a flood of attempts, and hashing bounds how many run at
	// once. Both exist because a password check is expensive on purpose and
	// reachable without a session.
	logins  *limiter
	hashing chan struct{}

	tpl *template.Template
	// assetVersion changes whenever an embedded asset changes, which is what
	// lets the stylesheet be cached forever and still update on a new release.
	assetVersion string
}

// New parses the templates and prepares the server.
func New(lib *library.Library, acc *account.Store, cfg config.Config, log *slog.Logger) (*Server, error) {
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
	return &Server{
		lib:          lib,
		acc:          acc,
		cfg:          cfg,
		log:          log,
		tpl:          tpl,
		assetVersion: version,
		logins:       newLimiter(),
		hashing:      make(chan struct{}, hashConcurrency),
	}, nil
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
// hashConcurrency bounds simultaneous password hashing. Two, so a login is never
// queued behind more than one other and a small machine keeps a core for serving
// pages.
const hashConcurrency = 2

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

	// Choosing and printing many songs at once. Reading, so public; linked from
	// the settings page because that is where somebody will look for it.
	mux.HandleFunc("GET /songbook", s.handleSongbook)
	mux.HandleFunc("GET /songbook/print", s.handleSongbookPrint)

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
	// Accounts. The login page is public, because that is what it is for.
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.openPost(s.handleLogin))
	// No GET /logout: a GET that changes state is exactly what SameSite=Lax does
	// not protect.
	mux.HandleFunc("POST /logout", s.openPost(s.handleLogout))
	mux.HandleFunc("GET /register", s.handleRegisterForm)
	mux.HandleFunc("POST /register", s.openPost(s.handleRegister))

	mux.HandleFunc("GET /settings", s.handleSettings)
	mux.HandleFunc("POST /settings/profile", s.authedPost(s.handleProfile))
	mux.HandleFunc("POST /settings/password", s.authedPost(s.handlePassword))
	mux.HandleFunc("POST /settings/look", s.authedPost(s.handleLook))
	mux.HandleFunc("POST /settings/sessions/revoke", s.authedPost(s.handleRevokeSessions))
	mux.HandleFunc("POST /settings/claim", s.openPost(s.handleClaim))

	mux.HandleFunc("GET /settings/users", s.handleUsers)
	mux.HandleFunc("POST /settings/users", s.adminPost(s.handleCreateUser))
	mux.HandleFunc("POST /settings/registration", s.adminPost(s.handleSetRegistration))
	mux.HandleFunc("GET /settings/users/{id}", s.handleUserEdit)
	mux.HandleFunc("POST /settings/users/{id}/name", s.adminPost(s.handleRenameUser))
	mux.HandleFunc("POST /settings/users/{id}/role", s.adminPost(s.handleSetRole))
	mux.HandleFunc("POST /settings/users/{id}/password", s.adminPost(s.handleResetPassword))
	mux.HandleFunc("POST /settings/users/{id}/delete", s.adminPost(s.handleDeleteUser))

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /static/{version}/{path...}", s.handleStatic)

	// A catch-all for anything unmatched, so a mistyped address gets Tabernacle's
	// own page with the navigation on it rather than a bare line of text with no
	// way back. Registered for GET only, which leaves the wrong method on a real
	// route answering 405 as it should.
	mux.HandleFunc("GET /", s.handleNotFound)

	// Outermost first. The host check comes before anything that reads the
	// request; the session is resolved once just outside the routing, so every
	// handler and every page can ask who is looking; and the panic guard wraps
	// everything so a bug is a 500 rather than a dead connection.
	var h http.Handler = mux
	h = s.authenticate(h)
	h = s.securityHeaders(h)
	h = s.requireAllowedHost(h)
	h = s.logRequests(h)
	h = s.recoverPanics(h)
	return h
}

// page is the common shell every template receives.
type page struct {
	// Print renders the interface's own words. Templates reach it as $.T, so a
	// page in the middle of a {{with}} can still ask for one.
	Print *i18n.Printer

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
	Sections []navLink // Canciones, Artistas, Géneros

	// Add is the primary action and lives in the header rather than among the
	// sections, because it is something you do, not somewhere you go. Nil when
	// nobody may write: read-only mode, or signed out of a claimed instance.
	Add *navLink

	// Account is the other header button: Acceder when signed out, Ajustes when
	// signed in. Never nil.
	Account *navLink
	// UserName is empty when signed out.
	UserName string
	IsAdmin  bool
	// Unclaimed drives the banner saying this instance has no administrator, and
	// therefore that anybody who can reach it can change the collection.
	Unclaimed bool
	// BodyClass carries a chosen theme onto <body>, where it beats the reader's
	// system preference.
	BodyClass string
	// LoginBox asks for the popover markup. Absent on the login page itself,
	// which is the form already.
	LoginBox  bool
	LoginNext string
	// Registration mirrors the setting, so the login box can offer the way to
	// make an account when there is one.
	Registration bool
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
// T renders one of the interface's messages.
func (p page) T(key string, args ...any) string { return p.Print.T(key, args...) }

// Lang is the language this page is in, for the html element and the chooser.
func (p page) Lang() string { return string(p.Print.Lang()) }

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
	case strings.HasPrefix(path, "/settings"), strings.HasPrefix(path, "/songbook"):
		section = "settings"
	}

	v := viewerOf(r)
	t := i18n.For(i18n.Lang(v.User.Lang))

	// Three sections and nothing else: these are the three ways to walk the
	// collection. Anything more and two labels start to look like each other,
	// which is how a reader ends up clicking the wrong one.
	nav := navState{
		Query:   qs.Get("q"),
		Section: section,
		Sections: []navLink{
			{Label: t.T("nav.songs"), URL: "/", On: section == "songs"},
			{Label: t.T("nav.artists"), URL: "/artists", On: section == "artists"},
			{Label: t.T("nav.genres"), URL: "/genres", On: section == "genres"},
		},
	}
	if !s.cfg.ReadOnly && v.MayWrite() {
		nav.Add = &navLink{Label: t.T("nav.add"), URL: "/songs/new", On: section == "add"}
	}
	if v.In {
		nav.Account = &navLink{Label: t.T("nav.settings"), URL: "/settings", On: section == "settings"}
		nav.UserName = v.User.Name
		nav.IsAdmin = v.User.IsAdmin()
		nav.BodyClass = themeClass(v.User.Theme)
	} else {
		nav.Account = &navLink{Label: t.T("nav.signin"), URL: "/login", On: path == "/login"}
		nav.LoginBox = path != "/login" && path != "/register"
		nav.LoginNext = safeNext(path)
		nav.Registration = s.acc.RegistrationOpen()
	}
	nav.Unclaimed = v.Unclaimed
	return page{Print: t, Title: title, Nav: nav, Body: body}
}

// render executes a template into a buffer before writing anything.
//
// Buffering matters: a template that fails halfway through would otherwise
// leave a truncated page already committed with a 200.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, p page) {
	// Every page now depends on who is asking — the header carries a name, and
	// /settings carries a user list — so none of them may be cached by anything
	// in between. They were never cacheable in practice; saying so is free.
	w.Header().Set("Cache-Control", "no-store")

	p.AssetVersion = s.assetVersion
	p.Nav.ReadOnly = s.cfg.ReadOnly
	if p.Nav.Flash == "" {
		if code := s.takeFlash(w, r); code != "" {
			p.Nav.Flash = p.T("flash." + code)
		}
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
	// Narrowest first: what this page asks for, then what was saved for this
	// song, then what the reader chose for themselves, then what the instance
	// says for somebody who has chosen nothing.
	pick := r.URL.Query().Get("n")
	if pick == "" {
		pick = stored
	}
	if pick == "" {
		pick = viewerOf(r).User.Notation
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

// simplifyFor reads whether the reader asked for chords they can play.
//
// A saved preference is honoured when the query says nothing, the same way the
// capo view and the notation are. An explicit "0" turns it off, so the control
// can be switched back with a plain link.
func simplifyFor(r *http.Request, stored bool) bool {
	switch r.URL.Query().Get("s") {
	case "1":
		return true
	case "0":
		return false
	}
	return stored
}
