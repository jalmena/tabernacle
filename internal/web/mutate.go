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

	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/render"
	"github.com/jalmena/tabernacle/internal/song"
)

// maxFormBytes caps a form submission. A song is a text file; anything larger is
// a mistake or an attack.
const maxFormBytes = 2 << 20

// flash codes. A message is chosen from this fixed set and looked up
// server-side, so nothing from a query string is ever rendered back to the page.
var flashes = map[string]string{
	"created":   "Canción guardada.",
	"saved":     "Cambios guardados.",
	"deleted":   "Canción movida a la papelera.",
	"committed": "Tono guardado en el archivo.",
	"reindexed": "Biblioteca revisada.",
	"scanning":  "Ya hay una revisión en curso.",
}

// flashFor resolves a message code, ignoring anything not in the list.
func flashFor(r *http.Request) string { return flashes[r.URL.Query().Get("msg")] }

// requireSameOrigin refuses a state-changing request that did not come from this
// site.
//
// There are no accounts, so there are no credentials for a SameSite cookie to
// protect and a cross-origin POST would otherwise simply work: a page on another
// site could delete a song by submitting a form. This check and the Host
// allowlist together are the whole defence, so it fails closed — a missing
// Origin on a mutation is refused rather than waved through.
func (s *Server) requireSameOrigin(r *http.Request) error {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none", "":
		// "none" is a user typing the address; "" is a client that does not send
		// the header, which the Origin check below still covers.
	default:
		return errors.New("cross-site request")
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		// Browsers always send Origin on a cross-origin POST, so an absent one
		// means a same-origin form or a non-browser client. Requiring the Host
		// to be allowlisted is what makes that safe.
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

// mutating wraps a handler that changes something.
//
// It is one place so that no write route can be added without the origin check,
// the read-only guard and the body limit.
func (s *Server) mutating(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.ReadOnly {
			s.fail(w, r, http.StatusForbidden, "Solo lectura",
				"Esta instancia está configurada para no permitir cambios en la colección.")
			return
		}
		if err := s.requireSameOrigin(r); err != nil {
			s.log.Warn("refused a cross-origin write",
				"path", r.URL.Path, "origin", r.Header.Get("Origin"), "host", r.Host,
				"reason", err)
			s.fail(w, r, http.StatusForbidden, "Petición rechazada",
				"La petición no parece venir de esta página.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
		if err := r.ParseForm(); err != nil {
			s.fail(w, r, http.StatusRequestEntityTooLarge, "Envío demasiado grande",
				"El formulario supera el tamaño permitido.")
			return
		}
		h(w, r)
	}
}

// mutatingMultipart is mutating for a handler that parses its own body.
//
// An upload cannot go through ParseForm, so the guards are applied and the body
// is left for the handler, which caps it itself.
func (s *Server) mutatingMultipart(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.ReadOnly {
			s.fail(w, r, http.StatusForbidden, "Solo lectura",
				"Esta instancia está configurada para no permitir cambios en la colección.")
			return
		}
		if err := s.requireSameOrigin(r); err != nil {
			s.log.Warn("refused a cross-origin upload",
				"origin", r.Header.Get("Origin"), "host", r.Host, "reason", err)
			s.fail(w, r, http.StatusForbidden, "Petición rechazada",
				"La petición no parece venir de esta página.")
			return
		}
		h(w, r)
	}
}

// redirect sends the browser onward after a change, so a refresh does not repeat
// it.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, target, msg string) {
	if msg != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "msg=" + url.QueryEscape(msg)
	}
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
}

func (s *Server) handleNewSong(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "editor.html", s.page(r, "Añadir una canción", editorBody{
		Title:       "Añadir una canción",
		Action:      "/songs",
		CancelURL:   "/",
		Placeholder: newSongPlaceholder,
	}))
}

func (s *Server) handleCreateSong(w http.ResponseWriter, r *http.Request) {
	text := r.PostFormValue("text")
	slug, err := s.lib.CreateFromText(r.Context(), text)
	if err != nil {
		s.renderEditorError(w, r, editorBody{
			Title:       "Añadir una canción",
			Action:      "/songs",
			CancelURL:   "/",
			Text:        text,
			Placeholder: newSongPlaceholder,
		}, err)
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
	body := editorBody{
		Title:       "Añadir una canción",
		Action:      "/songs",
		CancelURL:   "/",
		Placeholder: newSongPlaceholder,
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxSongBytes+(1<<16))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		body.ErrMessage = "No se ha podido leer el archivo, o es demasiado grande."
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
		body.ErrMessage = "No has seleccionado ningún archivo."
		s.render(w, r, http.StatusBadRequest, "editor.html", s.page(r, body.Title, body))
		return
	}

	f, err := files[0].Open()
	if err != nil {
		body.ErrMessage = "No se ha podido abrir el archivo."
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

	body.Text = strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	body.Loaded = safeName(files[0].Filename)
	// The text is shown, not saved. If it does not parse, the reader sees the
	// line to fix before anything reaches the collection.
	if _, err := song.Parse([]byte(body.Text)); err != nil {
		var se *song.SyntaxError
		if errors.As(err, &se) {
			body.ErrMessage = se.Msg
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
	s.render(w, r, http.StatusOK, "editor.html", s.page(r, "Editar", editorBody{
		Title:     "Editar el archivo",
		Action:    "/songs/" + slug,
		Slug:      slug,
		Text:      string(data),
		Hash:      hash,
		Editing:   true,
		CancelURL: "/songs/" + slug,
	}))
}

func (s *Server) handleSaveSong(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	text := r.PostFormValue("text")
	hash := r.PostFormValue("hash")

	newSlug, err := s.lib.SaveRaw(r.Context(), slug, text, hash)
	if err != nil {
		body := editorBody{
			Title:     "Editar el archivo",
			Action:    "/songs/" + slug,
			Slug:      slug,
			Text:      text,
			Hash:      hash,
			Editing:   true,
			CancelURL: "/songs/" + slug,
		}
		if errors.Is(err, library.ErrConflict) {
			// Both versions matter here, so the user is told rather than having
			// one of them quietly discarded.
			body.ErrMessage = "El archivo ha cambiado desde que abriste el editor. " +
				"Copia tu texto, vuelve a abrir la canción y aplica los cambios de nuevo."
			s.render(w, r, http.StatusConflict, "editor.html", s.page(r, "Conflicto", body))
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
	var se *song.SyntaxError
	if errors.As(err, &se) {
		body.ErrMessage = se.Msg
		body.ErrLine = se.Line
	} else {
		body.ErrMessage = err.Error()
	}
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
	s.render(w, r, http.StatusOK, "confirm.html", s.page(r, "Eliminar", confirmBody{
		Heading: "¿Eliminar " + name + "?",
		Detail: "El archivo se mueve a la carpeta de la papelera, dentro del directorio " +
			"de datos. Puedes recuperarlo desde ahí con el gestor de archivos.",
		Action:    "/songs/" + slug + "/delete",
		Confirm:   "Sí, eliminar",
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
		s.fail(w, r, http.StatusBadRequest, "Valor no válido",
			"El número de semitonos no es un número.")
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
		s.fail(w, r, http.StatusBadRequest, "Nada que guardar",
			"No hay ninguna transposición que escribir en el archivo.")
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
			s.render(w, r, http.StatusOK, "scan.html", s.page(r, "Revisión", scanBody{Report: last}))
			return
		}
		s.serverError(w, r, err)
		return
	}
	changed := rep.Added+rep.Updated+rep.Renamed+rep.Removed+rep.Failed > 0
	s.render(w, r, http.StatusOK, "scan.html",
		s.page(r, "Revisión", scanBody{Report: rep, Changed: changed}))
}
