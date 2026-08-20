package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	"created":     "Canción guardada.",
	"saved":       "Cambios guardados.",
	"deleted":     "Canción movida a la papelera.",
	"committed":   "Tono guardado en el archivo.",
	"favorited":   "Añadida a favoritas.",
	"unfavorited": "Quitada de favoritas.",
	"reindexed":   "Biblioteca revisada.",
	"scanning":    "Ya hay una revisión en curso.",
}

// flashFor resolves a message code, ignoring anything not in the list.
func flashFor(r *http.Request) string { return flashes[r.URL.Query().Get("msg")] }

// requireSameOrigin refuses a state-changing request that did not come from this
// site.
//
// With no accounts there are no credentials for SameSite cookies to protect, so
// a cross-origin POST would otherwise simply work: a page on another site could
// delete a song by submitting a form. This check and the Host allowlist together
// are the whole defence, so it fails closed — a missing Origin on a mutation is
// refused rather than waved through.
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
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return errors.New("unparseable origin")
	}
	if !strings.EqualFold(u.Host, r.Host) && !s.cfg.HostAllowed(u.Host) {
		return errors.New("foreign origin")
	}
	return nil
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

// editorBody is what the create and edit forms receive.
type editorBody struct {
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
	s.render(w, r, http.StatusOK, "editor.html", page_("Añadir una canción", "", editorBody{
		Title:     "Añadir una canción",
		Action:    "/songs",
		CancelURL: "/",
		Text:      "",
	}))
}

func (s *Server) handleCreateSong(w http.ResponseWriter, r *http.Request) {
	text := r.PostFormValue("text")
	slug, err := s.lib.CreateFromText(r.Context(), text)
	if err != nil {
		s.renderEditorError(w, r, editorBody{
			Title:     "Añadir una canción",
			Action:    "/songs",
			CancelURL: "/",
			Text:      text,
		}, err)
		return
	}
	s.redirect(w, r, "/songs/"+slug, "created")
}

func (s *Server) handleEditSong(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	data, hash, _, err := s.lib.RawFile(r.Context(), slug)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "editor.html", page_("Editar", "", editorBody{
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
			s.render(w, r, http.StatusConflict, "editor.html", page_("Conflicto", "", body))
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
		page_(body.Title, "", body))
}

func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	on := r.PostFormValue("on") == "1"
	if err := s.lib.SetFavorite(r.Context(), slug, on); err != nil {
		s.resolve(w, r, err)
		return
	}
	msg := "unfavorited"
	if on {
		msg = "favorited"
	}
	s.redirect(w, r, "/songs/"+slug, msg)
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

func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	if _, err := s.lib.Sync(r.Context()); err != nil {
		if errors.Is(err, library.ErrSyncBusy) {
			s.redirect(w, r, "/", "scanning")
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/", "reindexed")
}
