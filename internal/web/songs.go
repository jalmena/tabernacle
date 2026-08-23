package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/render"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// songBody is what the song templates receive.
type songBody struct {
	Slug  string
	Sheet render.SheetView
	// Theme and Themes drive the print view's paper choice.
	Theme    string
	Themes   []themeChoice
	Controls controlSet
	Broken   bool
	ParseMsg string
	ParseAt  int
	ReadOnly bool
	// CanEdit is whether this reader may change the collection: not read-only,
	// and either signed in or looking at an instance nobody has claimed. The
	// editing controls are hidden otherwise, because offering a button that
	// answers 401 is worse than not offering it.
	CanEdit  bool
	PrintURL string
	RawURL   string
	EditURL  string
}

// controlSet is the transposition and notation bar.
//
// Every field is a finished URL, built in Go. Assembling these in a template
// would mean escaping query parameters by hand in the one place it is easiest
// to get wrong.
type controlSet struct {
	DownURL  string
	UpURL    string
	ResetURL string
	CanReset bool

	Transpose int
	KeyLabel  string
	OrigKey   string
	StepLabel string
	// StepShort is "+2" or "-1", empty at the original key. It sits next to the
	// note between the two buttons, which is where a reader is already looking.
	StepShort string

	NotationURL   string
	NotationLabel string

	// The simplify control appears only when it would change something.
	ShowSimplify bool
	SimplifyURL  string
	SimplifyOn   bool
}

// themeChoice is one paper option offered inside the print view.
type themeChoice struct {
	ID    string
	Label string
	URL   string
	On    bool
}

// printThemes are the papers a sheet can be printed on.
//
// All of them are light. A dark sheet on paper costs a cartridge and reads worse
// than the white one, so there is no dark option here at all: the choice is
// between shades of nearly-white and how heavy the ink is.
var printThemeList = []struct{ id, label string }{
	{"white", "Blanco"},
	{"cream", "Crema"},
	{"grey", "Gris claro"},
	{"contrast", "Alto contraste"},
}

func printThemes(cur songView) []themeChoice {
	current := cur.Theme
	if current == "" {
		current = "white"
	}
	out := make([]themeChoice, 0, len(printThemeList))
	for _, t := range printThemeList {
		v := cur
		v.Theme = t.id
		out = append(out, themeChoice{
			ID: t.id, Label: t.label, URL: v.printing().URL(), On: t.id == current,
		})
	}
	return out
}

// printThemeFor reads the requested paper, falling back to plain white.
func printThemeFor(r *http.Request) string {
	want := r.URL.Query().Get("theme")
	for _, t := range printThemeList {
		if t.id == want {
			return want
		}
	}
	return "white"
}

// loadSong assembles everything both the full page and the fragment need.
func (s *Server) loadSong(r *http.Request) (songBody, songView, error) {
	slug := r.PathValue("slug")
	view, err := s.lib.Get(r.Context(), slug)
	if err != nil {
		return songBody{}, songView{}, err
	}

	notation := s.notationFor(r, view.State.Notation)
	transpose := transposeFor(r, view.State.Transpose)
	simplify := simplifyFor(r, view.State.Simplify)

	tuning, _ := voicing.ParseTuning(view.Song.Tuning)
	sheet := render.Sheet(view.Song, render.Options{
		Transpose: transpose,
		Notation:  notation,
		Lang:      s.langFor(r),
		Diagrams:  true,
		Tuning:    tuning,
		Simplify:  simplify,
	})

	notationName := config.NotationEnglish
	if sheet.Notation == "solfege" {
		notationName = config.NotationSolfege
	}
	cur := songView{
		Slug:            slug,
		Theme:           printThemeFor(r),
		Transpose:       transpose,
		Notation:        notationName,
		Simplify:        simplify,
		DefaultNotation: s.defaultNotation(view.State.Notation),
	}

	body := songBody{
		Slug:     slug,
		Sheet:    sheet,
		Theme:    printThemeFor(r),
		Themes:   printThemes(cur),
		ReadOnly: s.cfg.ReadOnly,
		CanEdit:  !s.cfg.ReadOnly && viewerOf(r).MayWrite(),
		PrintURL: cur.printing().URL(),
		RawURL:   "/songs/" + slug + "/raw",
		EditURL:  "/songs/" + slug + "/edit",
		Controls: controlSet{
			DownURL:  cur.withTranspose(transpose - 1).URL(),
			UpURL:    cur.withTranspose(transpose + 1).URL(),
			ResetURL: cur.withTranspose(0).URL(),
			CanReset: transpose != 0,

			Transpose: transpose,
			KeyLabel:  sheet.Key,
			OrigKey:   sheet.OrigKey,
			StepLabel: stepLabel(transpose),
			StepShort: stepShort(transpose),

			NotationURL:   cur.withNotation(otherNotation(notationName)).URL(),
			NotationLabel: notationLabel(notationName),
		},
	}
	// Offered when it is already on, so it can be turned off, or when turning it
	// on would actually rewrite something.
	body.Controls.ShowSimplify = sheet.Simplified || sheet.CanSimplify
	body.Controls.SimplifyOn = sheet.Simplified
	if body.Controls.ShowSimplify {
		body.Controls.SimplifyURL = cur.withSimplify(!simplify).URL()
	}

	if view.ParseError != nil {
		body.Broken = true
		body.ParseMsg, body.ParseAt = describeSyntax(view.ParseError)
	}
	return body, cur, nil
}

// defaultNotation is what this reader sees when a URL says nothing about note
// names: their own saved preference if they have one, otherwise the deployment's.
func (s *Server) defaultNotation(stored string) string {
	if stored != "" {
		return stored
	}
	if s.cfg.Notation == config.NotationSolfege {
		return config.NotationSolfege
	}
	return config.NotationEnglish
}

// stepLabel names the current transposition the way a musician would say it.
func stepLabel(n int) string {
	switch {
	case n == 0:
		return "tono original"
	case n == 1:
		return "+1 semitono"
	case n == -1:
		return "−1 semitono"
	case n > 0:
		return "+" + strconv.Itoa(n) + " semitonos"
	default:
		return "−" + strconv.Itoa(-n) + " semitonos"
	}
}

// stepShort is the signed number of semitones, or "" at the original key.
func stepShort(n int) string {
	switch {
	case n == 0:
		return ""
	case n > 0:
		return "+" + strconv.Itoa(n)
	default:
		return "\u2212" + strconv.Itoa(-n) // a real minus sign, not a hyphen
	}
}

// notationLabel names the notation the button switches TO, not the one in use.
//
// A button labelled with what you are already looking at reads as a statement
// and leaves you guessing what pressing it does; labelled with the destination,
// it reads as an instruction.
func notationLabel(current string) string {
	if current == config.NotationSolfege {
		return "C D E"
	}
	return "Do Re Mi"
}

// describeSyntax pulls a message and a line number out of a parse failure, so
// the page can point at the line to fix rather than just saying "broken".
func describeSyntax(err error) (string, int) {
	var se *song.SyntaxError
	if errors.As(err, &se) {
		return se.Msg, se.Line
	}
	return err.Error(), 0
}

// resolve turns a library error into a response, handling the redirect a renamed
// song needs so a bookmark keeps working.
func (s *Server) resolve(w http.ResponseWriter, r *http.Request, err error) {
	var moved library.ErrMoved
	switch {
	case errors.As(err, &moved):
		target := "/songs/" + moved.NewSlug
		if q := r.URL.RawQuery; q != "" {
			target += "?" + q
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	case errors.Is(err, library.ErrNotFound):
		s.fail(w, r, http.StatusNotFound, "Esa canción no está aquí",
			"Puede que se haya renombrado o eliminado. Prueba a buscarla desde la biblioteca.")
	default:
		s.serverError(w, r, err)
	}
}

func (s *Server) handleSong(w http.ResponseWriter, r *http.Request) {
	body, _, err := s.loadSong(r)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	// Opening a song is what feeds the recently-played list. It is deliberately
	// not recorded for the fragment, the print view or a raw download, so
	// transposing a song does not count as opening it four more times.
	s.lib.RecordView(r.Context(), body.Slug)

	t := body.Sheet.Title
	if body.Sheet.Artist != "" {
		t += " · " + body.Sheet.Artist
	}
	s.render(w, r, http.StatusOK, "song.html", s.page(r, t, body))
}

// handleSheetFragment returns just the sheet and its controls, for the
// scroll-preserving swap.
//
// It renders through exactly the same template as the full page, so the two can
// never drift apart.
func (s *Server) handleSheetFragment(w http.ResponseWriter, r *http.Request) {
	body, _, err := s.loadSong(r)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "fragment.html", s.page(r, "", body))
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	body, _, err := s.loadSong(r)
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "print.html", s.page(r, body.Sheet.Title, body))
}

// handleRaw serves the .song file itself.
//
// This is the escape hatch that keeps the format honest: whatever the interface
// can do, the file is still there to be downloaded, edited in a text editor and
// kept.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	data, _, filename, err := s.lib.RawFile(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.resolve(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// nosniff plus a text type is what keeps a browser from ever deciding this
	// is HTML and running it.
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// contentDisposition builds an attachment header that survives a non-ASCII
// filename.
//
// The plain filename parameter is reduced to ASCII for old clients, and the
// RFC 5987 form carries the real one, so "Y nos dieron las diez.song" downloads
// under its own name.
func contentDisposition(name string) string {
	ascii := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r < 32 || r == 127 || r == '"' || r == '\\':
			// Dropped: a quote or a control character here is how this header
			// gets split.
		case r > 127:
			ascii = append(ascii, '_')
		default:
			ascii = append(ascii, r)
		}
	}
	fallback := string(ascii)
	if fallback == "" {
		fallback = "song.song"
	}
	return "attachment; filename=\"" + fallback + "\"; filename*=UTF-8''" + urlEscapePath(name)
}

// urlEscapePath percent-encodes everything that is not unreserved, as the
// extended filename parameter requires.
func urlEscapePath(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}
