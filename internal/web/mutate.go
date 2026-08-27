package web

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/render"
	"github.com/jalmena/tabernacle/internal/song"
)

// maxFormBytes caps a form submission. A song is a text file; anything larger is
// a mistake or an attack.
const maxFormBytes = 2 << 20

// flashCodes is every notice the application can leave for the next page. The
// words live in the message catalogue; this is only the list of what may be
// asked for, so nothing a browser sends can reach the page.
var flashCodes = map[string]bool{
	"created": true, "saved": true, "deleted": true, "committed": true,
	"reindexed": true, "scanning": true,

	"welcome": true, "bye": true, "claimed": true, "registered": true,
	"profile": true, "password": true, "logged-out": true, "language": true,

	"user-created": true, "user-deleted": true, "role-changed": true,
	"user-reset": true, "notation": true,
	"registration-open": true, "registration-closed": true,

	"none-picked": true,
}

// flashCookie carries a one-shot message across a redirect.
//
// A query parameter would do the same job and put "?msg=logged-out" in the
// address bar, where it stays in the history, in a shared link and in a
// bookmark. A cookie that is read once and cleared leaves the address alone.
const flashCookie = "tabernacle_flash"

// setFlash remembers a message for the next page this browser asks for.
func (s *Server) setFlash(w http.ResponseWriter, r *http.Request, code string) {
	if !flashCodes[code] {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    code,
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.overTLS(r),
	})
}

// takeFlash reads the notice's code and clears it, so a reload does not repeat
// it.
func (s *Server) takeFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.overTLS(r),
	})
	// Only ever a code from the fixed list above; nothing a browser sent is
	// rendered back to the page.
	if !flashCodes[c.Value] {
		return ""
	}
	return c.Value
}

// requireSameOrigin refuses a state-changing request that did not come from this
// site.
//
// A session cookie is SameSite=Lax, which keeps it off a cross-site POST, and
// this check is what covers everything else: an unclaimed instance has no
// credential to protect at all, and Lax still allows a top-level cross-site GET.
// It fails closed — a missing Origin on a mutation is refused rather than waved
// through — because a write that cannot be shown to come from this page is not
// worth guessing about.
func (s *Server) requireSameOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	// "null" is a browser declining to say. It is not a host, and treating it as
	// one refuses the request it was trying to describe.
	told := origin != "" && origin != "null"

	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		// The browser is telling us where this came from, and page content cannot
		// set this header: it is a forbidden header name, so neither a form nor
		// fetch can forge it. That makes it better evidence than Origin, which
		// Firefox reduces to "null" on a POST whenever the referrer policy is
		// restrictive — and every response here carries no-referrer.
		//
		// "none" is somebody typing an address or following a bookmark.
		//
		// A contradictory Origin still loses: if the browser named one, it has to
		// agree.
		if told && !s.originAllowed(origin, r.Host) {
			return errors.New("foreign origin")
		}
		return nil
	case "":
		// An old browser, or something that is not a browser. Fall through to the
		// Origin check, which is all the evidence there is.
	default:
		return errors.New("cross-site request")
	}

	if !told {
		// Nothing said where this came from. The Host allowlist is what makes
		// that safe: a page on a public name cannot make a browser claim to be
		// talking to a private one.
		if !s.cfg.HostAllowed(r.Host) {
			return errors.New("host not allowed")
		}
		return nil
	}
	if !s.originAllowed(origin, r.Host) {
		return errors.New("foreign origin")
	}
	return nil
}

// originAllowed reports whether a page served from this origin may submit here.
//
// The port is part of the comparison, which is what separates this from
// HostAllowed. HostAllowed answers "which names does this server respond to",
// and a permissive policy is right for that: a home server is reached by half a
// dozen names it cannot enumerate. Answering "which pages may submit to it" that
// way is wrong, because a browser scopes neither an origin nor a cookie by
// hostname alone. Another service on the same machine, or on the same LAN name
// at a different port, is somebody else.
func (s *Server) originAllowed(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, host) {
		return true
	}
	// A reverse proxy that rewrites Host: the browser's origin is the public
	// name, and BaseURL is where that name is recorded.
	if s.cfg.BaseURL != nil && strings.EqualFold(u.Host, s.cfg.BaseURL.Host) {
		return true
	}
	// An explicitly allowlisted name, on the port we are actually being
	// addressed on. The list holds bare names, so the port is compared against
	// the request's rather than folded away.
	originHost, originPort := splitHostPort(u.Host)
	_, wantPort := splitHostPort(host)
	if originPort != wantPort {
		return false
	}
	for _, a := range s.cfg.AllowedHosts {
		if strings.EqualFold(originHost, a) {
			return true
		}
	}
	return false
}

// splitHostPort separates a host from its port, tolerating both an absent port
// and an IPv6 literal.
func splitHostPort(hostport string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]"), ""
}

// access is the least a request must be to get through a guard.
type access int

const (
	// accessPublic asks nothing. Login and logout, and nothing else.
	accessPublic access = iota
	// accessWriter is any signed-in account, or anybody at all while the
	// instance is unclaimed.
	accessWriter
	// accessAdmin is a signed-in administrator. Never satisfied by an unclaimed
	// instance: claiming one is its own route with its own rule.
	accessAdmin
)

// guard is what a write route demands before its handler runs.
type guard struct {
	// collection marks a route that changes songs, and therefore one that
	// read-only mode refuses.
	collection bool
	minimum    access
	// parseForm reads an ordinary form. Multipart handlers cap and read their own
	// bodies instead.
	parseForm bool
}

// guarded is the one place a write can be refused, so that a new write route
// cannot be added without a read-only guard, an origin check, a privilege and a
// body limit.
//
// The order is deliberate. Read-only comes first and unchanged, so a read-only
// instance stays read-only whoever is signed in. The body limit comes last, so an
// anonymous request never gets its body read.
func (s *Server) guarded(g guard, h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.collection && s.cfg.ReadOnly {
			s.fail(w, r, http.StatusForbidden, "fail.readonly", "fail.readonly.detail")
			return
		}
		if err := s.requireSameOrigin(r); err != nil {
			s.log.Warn("refused a cross-origin write",
				"path", r.URL.Path, "origin", r.Header.Get("Origin"), "host", r.Host,
				"fetch_site", r.Header.Get("Sec-Fetch-Site"), "reason", err)
			s.fail(w, r, http.StatusForbidden, "fail.origin", "fail.origin.detail")
			return
		}
		if !s.permitted(r, g.minimum) {
			s.refusePrivilege(w, r, g.minimum)
			return
		}
		if g.parseForm {
			r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
			if err := r.ParseForm(); err != nil {
				s.fail(w, r, http.StatusRequestEntityTooLarge, "fail.toobig", "fail.toobig.detail")
				return
			}
		}
		h(w, r)
	}
}

func (s *Server) permitted(r *http.Request, need access) bool {
	v := viewerOf(r)
	switch need {
	case accessAdmin:
		return v.MayAdmin()
	case accessWriter:
		return v.MayWrite()
	default:
		return true
	}
}

// refusePrivilege answers with the thing the reader can actually do about it.
//
// Signed out, the next step is to sign in, so the login page is what they get
// rather than a dead end that says no.
func (s *Server) refusePrivilege(w http.ResponseWriter, r *http.Request, need access) {
	if !viewerOf(r).In {
		body := loginBody{
			Next:    safeNext(r.URL.Path),
			Problem: i18n.For(s.langFor(r)).T("login.needed"),
		}
		s.render(w, r, http.StatusUnauthorized, "login.html", s.page(r, "login.title", body))
		return
	}
	_ = need
	s.fail(w, r, http.StatusForbidden, "fail.notallowed", "fail.notallowed.detail")
}

// mutating wraps a handler that changes the collection.
func (s *Server) mutating(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return s.guarded(guard{collection: true, minimum: accessWriter, parseForm: true}, h)
}

// mutatingMultipart wraps a handler that changes the collection and reads its own
// body, because a multipart upload is capped by the handler that understands it.
func (s *Server) mutatingMultipart(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return s.guarded(guard{collection: true, minimum: accessWriter}, h)
}

// adminPost wraps a handler that manages accounts or settings. Read-only mode
// governs the collection, not the application, so it does not apply.
func (s *Server) adminPost(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return s.guarded(guard{minimum: accessAdmin, parseForm: true}, h)
}

// authedPost wraps a handler that changes the caller's own account.
func (s *Server) authedPost(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return s.guarded(guard{minimum: accessWriter, parseForm: true}, h)
}

// openPost wraps a handler anybody may reach: logging in, logging out, and
// claiming an instance that has no accounts.
func (s *Server) openPost(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return s.guarded(guard{minimum: accessPublic, parseForm: true}, h)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, target, msg string) {
	s.setFlash(w, r, msg)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// newSongPlaceholder shows the whole header a song may carry.
//
// A field nobody knows about might as well not exist, and the format is small
// enough that showing all of it costs nothing. Only the title is required; the
// rest is there to be deleted.
const newSongPlaceholder = `---
title: Título de la canción
artist: Artista
key: Am
capo: 2
tuning: EADGBE
genre: bolero
notes: |-
  Lo que quieras recordar sobre la canción.
  Puede ocupar varias líneas.
---

## Estrofa 1
A[Am]quí van las [F]letras, con los acordes
donde de verdad [C]caen al can[G]tar

## Estribillo
[F]Otra sección, con su [C]nombre

## Solo
` + "```" + `tab
e|-------------------|
B|-------------------|
G|-------0-----------|
D|---0-------0---2---|
A|-------------------|
E|-3-----------------|
` + "```" + `
`

// editorBody is what the create and edit forms receive.
type editorBody struct {
	Placeholder string
	// Loaded names the file whose text is sitting in the box, so the reader can
	// tell that the upload arrived.
	Loaded     string
	Title      string // page heading
	Action     string // where the form posts
	Slug       string
	Text       string // what is in the textarea
	Hash       string // the version being edited, for conflict detection
	Editing    bool
	ErrMessage string
	ErrLine    int
	CancelURL  string

	// Compose is the song shown as fields and sections. Plain says the reader
	// asked for the file itself instead, which is the escape hatch and the way
	// to reach anything the fields cannot express.
	Compose composed
	Kinds   []kindChoice
}

// kindChoice is one button on the add-a-section row.
type kindChoice struct {
	Kind string
	Name string
}

// kindChoices are the sections offered, named in the reader's language.
func kindChoices(say *i18n.Printer) []kindChoice {
	out := make([]kindChoice, 0, len(sectionKinds))
	for _, k := range sectionKinds {
		out = append(out, kindChoice{Kind: string(k), Name: say.T("section." + string(k))})
	}
	return out
}

func (s *Server) handleNewSong(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	s.render(w, r, http.StatusOK, "editor.html", s.page(r, "library.empty.add", editorBody{
		Title:       say.T("library.empty.add"),
		Action:      "/songs",
		CancelURL:   "/",
		Placeholder: newSongPlaceholder,
		Kinds:       kindChoices(say),
	}))
}

func (s *Server) handleCreateSong(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	body := editorBody{
		Title:       say.T("library.empty.add"),
		Action:      "/songs",
		CancelURL:   "/",
		Placeholder: newSongPlaceholder,
		Kinds:       kindChoices(say),
	}
	c := readCompose(r)
	if act(r, &c, say) {
		// A section added, removed or moved, or the reader asking for the file
		// instead of the fields. Nothing is written: the page comes back with
		// the change made, which is how this works with no script at all.
		body.Compose, body.Text = c, c.text()
		s.render(w, r, http.StatusOK, "editor.html", s.page(r, "library.empty.add", body))
		return
	}

	text := c.text()
	body.Compose, body.Text = c, text
	slug, err := s.lib.CreateFromText(r.Context(), text)
	if err != nil {
		s.renderEditorError(w, r, body, err)
		return
	}
	s.redirect(w, r, "/songs/"+slug, "created")
}

// handleLoadIntoEditor takes an uploaded .song file and puts its text in the
// editor, without saving anything.
//
// It is a round trip rather than a purely local read so that it works with
// scripting off; app.js shortcuts it with a FileReader when it can. Either way
// nothing is written until the reader presses Guardar, which is the point: an
// upload you cannot see before it lands is how a collection fills with files
// nobody meant to add.
func (s *Server) handleLoadIntoEditor(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	body := editorBody{
		Title:       say.T("library.empty.add"),
		Action:      "/songs",
		CancelURL:   "/",
		Placeholder: newSongPlaceholder,
		Kinds:       kindChoices(say),
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxSongBytes+(1<<16))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		body.ErrMessage = say.T("editor.unreadable")
		s.render(w, r, http.StatusRequestEntityTooLarge, "editor.html",
			s.page(r, body.Title, body))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		body.ErrMessage = say.T("editor.nofile")
		s.render(w, r, http.StatusBadRequest, "editor.html", s.page(r, body.Title, body))
		return
	}

	f, err := files[0].Open()
	if err != nil {
		body.ErrMessage = say.T("editor.cannotopen")
		s.render(w, r, http.StatusBadRequest, "editor.html", s.page(r, body.Title, body))
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxSongBytes+1))
	if err != nil || int64(len(data)) > s.cfg.MaxSongBytes {
		body.ErrMessage = "El archivo es demasiado grande."
		s.render(w, r, http.StatusRequestEntityTooLarge, "editor.html",
			s.page(r, body.Title, body))
		return
	}
	if !utf8.Valid(data) {
		body.ErrMessage = "El archivo no es texto: un .song es texto plano."
		s.render(w, r, http.StatusUnprocessableEntity, "editor.html",
			s.page(r, body.Title, body))
		return
	}

	body.Text = newlines(string(data))
	// Shown as fields when the file can be taken apart, and as the file itself
	// when it cannot — which for an upload is the useful half of the time, since
	// a file somebody is loading is often one they are trying to fix.
	if p, err := song.Take([]byte(body.Text)); err == nil {
		body.Compose = composed{Parts: p}
	} else {
		body.Compose = composed{Plain: true, Raw: body.Text}
	}
	body.Loaded = safeName(files[0].Filename)
	// The text is shown, not saved. If it does not parse, the reader sees the
	// line to fix before anything reaches the collection.
	if _, err := song.Parse([]byte(body.Text)); err != nil {
		var se *song.SyntaxError
		if errors.As(err, &se) {
			body.ErrMessage = say.T(se.Key, se.Args...)
			body.ErrLine = se.Line
		} else {
			body.ErrMessage = err.Error()
		}
	}
	s.render(w, r, http.StatusOK, "editor.html", s.page(r, body.Title, body))
}

func (s *Server) handleEditSong(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	data, hash, _, err := s.lib.RawFile(r.Context(), slug)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	say := i18n.For(s.langFor(r))
	body := editorBody{
		Title:     say.T("editor.edit"),
		Action:    "/songs/" + slug,
		Slug:      slug,
		Text:      string(data),
		Hash:      hash,
		Editing:   true,
		CancelURL: "/songs/" + slug,
		Kinds:     kindChoices(say),
	}
	// Shown as fields when it can be taken apart, and as the file when it
	// cannot. A song that will not parse is exactly the one somebody needs the
	// file for, so that is where they are put, with the mistake in front of them.
	//
	// The collection does not normally hold one — a file that fails to parse is
	// recorded as broken rather than offered for editing — so this is a guard
	// against the day the two disagree, not a path anybody walks.
	if p, err := song.Take(data); err == nil {
		body.Compose = composed{Parts: p}
	} else {
		body.Compose = composed{Plain: true, Raw: string(data)}
	}
	s.render(w, r, http.StatusOK, "editor.html", s.page(r, "title.edit", body))
}

func (s *Server) handleSaveSong(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	slug := r.PathValue("slug")
	hash := r.PostFormValue("hash")

	c := readCompose(r)
	body := editorBody{
		Title:     say.T("editor.edit"),
		Action:    "/songs/" + slug,
		Slug:      slug,
		Hash:      hash,
		Editing:   true,
		CancelURL: "/songs/" + slug,
		Kinds:     kindChoices(say),
	}
	if act(r, &c, say) {
		body.Compose, body.Text = c, c.text()
		s.render(w, r, http.StatusOK, "editor.html", s.page(r, "editor.edit", body))
		return
	}

	text := c.text()
	body.Compose, body.Text = c, text
	newSlug, err := s.lib.SaveRaw(r.Context(), slug, text, hash)
	if err != nil {
		if errors.Is(err, library.ErrConflict) {
			// Both versions matter here, so the user is told rather than having
			// one of them quietly discarded.
			body.ErrMessage = i18n.For(s.langFor(r)).T("editor.conflict")
			s.render(w, r, http.StatusConflict, "editor.html", s.page(r, "title.conflict", body))
			return
		}
		s.renderEditorError(w, r, body, err)
		return
	}
	s.redirect(w, r, "/songs/"+newSlug, "saved")
}

// renderEditorError re-renders the form with the user's text intact and the
// line number of the mistake, which is the whole point of having a raw editor.
func (s *Server) renderEditorError(w http.ResponseWriter, r *http.Request, body editorBody, err error) {
	body.ErrMessage, body.ErrLine = describeSyntax(i18n.For(s.langFor(r)), err)
	s.render(w, r, http.StatusUnprocessableEntity, "editor.html",
		s.page(r, body.Title, body))
}

// confirmBody is a question with two answers.
type confirmBody struct {
	Heading   string
	Detail    string
	Action    string
	Confirm   string
	CancelURL string
}

// handleDeleteForm asks before removing a song.
//
// A page rather than a scripted dialog, because the content security policy
// rules out inline handlers and because a real page works with scripting off. It
// also gives room to say where the file is going, which is the reassuring part.
func (s *Server) handleDeleteForm(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	slug := r.PathValue("slug")
	view, err := s.lib.Get(r.Context(), slug)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	name := view.Record.Title
	if view.Record.Artist != "" {
		name += " · " + view.Record.Artist
	}
	s.render(w, r, http.StatusOK, "confirm.html", s.page(r, "song.delete", confirmBody{
		Heading:   say.T("delete.heading", name),
		Detail:    say.T("delete.detail"),
		Action:    "/songs/" + slug + "/delete",
		Confirm:   say.T("delete.confirm"),
		CancelURL: "/songs/" + slug,
	}))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.lib.Delete(r.Context(), r.PathValue("slug")); err != nil {
		s.resolve(w, r, err)
		return
	}
	s.redirect(w, r, "/", "deleted")
}

// handleRememberTranspose saves the reader's key for this song. The file is not
// touched.
func (s *Server) handleRememberTranspose(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	n, err := strconv.Atoi(r.PostFormValue("t"))
	if err != nil {
		s.fail(w, r, http.StatusBadRequest, "fail.badvalue", "fail.badvalue.detail")
		return
	}
	state, err := s.lib.State(r.Context(), slug)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	state.Transpose = render.NormalizeTranspose(n)
	if err := s.lib.SaveState(r.Context(), slug, state); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/songs/"+slug, "saved")
}

// handleCommitTranspose writes the transposition into the file, which is the
// deliberate counterpart to transposing merely to read.
func (s *Server) handleCommitTranspose(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	n, err := strconv.Atoi(r.PostFormValue("t"))
	if err != nil || render.NormalizeTranspose(n) == 0 {
		s.fail(w, r, http.StatusBadRequest, "fail.nothing", "fail.nothing.detail")
		return
	}
	if err := s.lib.CommitTranspose(r.Context(), slug, render.NormalizeTranspose(n)); err != nil {
		s.resolve(w, r, err)
		return
	}
	s.redirect(w, r, "/songs/"+slug, "committed")
}

// scanBody reports what a scan did.
type scanBody struct {
	Report  library.SyncReport
	Changed bool
}

// handleReindex rescans the collection and says what changed.
//
// It renders the result instead of redirecting with a one-line message. When
// nothing has changed there is otherwise no visible difference at all, and a
// button that reports nothing is indistinguishable from a button that does
// nothing — which is exactly how this one read.
func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	rep, err := s.lib.Sync(r.Context())
	if err != nil {
		if errors.Is(err, library.ErrSyncBusy) {
			// Another scan is already doing the work; report its result.
			last, _ := s.lib.LastScan()
			s.render(w, r, http.StatusOK, "scan.html", s.page(r, "scan.title", scanBody{Report: last}))
			return
		}
		s.serverError(w, r, err)
		return
	}
	changed := rep.Added+rep.Updated+rep.Renamed+rep.Removed+rep.Failed > 0
	s.render(w, r, http.StatusOK, "scan.html",
		s.page(r, "scan.title", scanBody{Report: rep, Changed: changed}))
}
