package web

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/library"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/songfs"
)

// maxUploadMemory is how much of a multipart form is held in memory before the
// rest spills to a temporary file.
const maxUploadMemory = 8 << 20

func (s *Server) handleImportForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "import.html", s.page(r, "transfer.title", importBody{
		Tabs:     s.settingsTabs(r, "import"),
		MaxBytes: s.cfg.MaxUploadBytes,
	}))
}

// importBody is what the import page receives, before and after an upload.
type importBody struct {
	Tabs     []navLink
	MaxBytes int64
	Report   *importView
	Problem  string
	// Artist stays in its field after an import, since the next batch is
	// usually more of the same artist.
	Artist string
}

// importView is the report with its reasons put into words.
//
// The library reports in codes, because it cannot know who is reading; this is
// where they become sentences. It is also the only place that knows, so the
// template is handed finished text and does no lookups of its own.
type importView struct {
	Added, Failed, Skipped []importRow
	Total                  int
}

type importRow struct {
	Name   string
	Title  string
	Slug   string
	Reason string
	Line   int
}

func viewOf(say *i18n.Printer, rep *library.ImportReport) *importView {
	if rep == nil {
		return nil
	}
	rows := func(in []library.ImportResult) []importRow {
		out := make([]importRow, 0, len(in))
		for _, r := range in {
			reason := ""
			if r.Key != "" {
				reason = say.T(r.Key, r.Args...)
			}
			out = append(out, importRow{
				Name: r.Name, Title: r.Title, Slug: r.Slug, Reason: reason, Line: r.Line,
			})
		}
		return out
	}
	return &importView{
		Added:   rows(rep.Added),
		Failed:  rows(rep.Failed),
		Skipped: rows(rep.Skipped),
		Total:   rep.Total(),
	}
}

// handleImport takes uploaded .song files and zip archives.
//
// Everything here treats its input as hostile: the request body is capped before
// anything reads it, each file is validated and parsed before a byte is written,
// and the destination on disk comes from the parsed metadata rather than from any
// name the upload supplied.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		s.render(w, r, http.StatusRequestEntityTooLarge, "import.html",
			s.page(r, "transfer.title", importBody{
				Tabs:     s.settingsTabs(r, "import"),
				MaxBytes: s.cfg.MaxUploadBytes,
				Problem:  i18n.For(s.langFor(r)).T("import.toobig"),
			}))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	var combined library.ImportReport
	say := i18n.For(s.langFor(r))
	artist := strings.TrimSpace(r.PostFormValue("artist"))
	verse := say.T("section.verse")
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		s.render(w, r, http.StatusBadRequest, "import.html", s.page(r, "transfer.title", importBody{
			Tabs:     s.settingsTabs(r, "import"),
			MaxBytes: s.cfg.MaxUploadBytes,
			Problem:  i18n.For(s.langFor(r)).T("editor.nofile"),
		}))
		return
	}

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			combined.Failed = append(combined.Failed, library.ImportResult{
				Name: safeName(fh.Filename), Key: "why.unreadable"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes+1))
		f.Close()
		if err != nil || int64(len(data)) > s.cfg.MaxUploadBytes {
			combined.Failed = append(combined.Failed, library.ImportResult{
				Name: safeName(fh.Filename), Key: "why.toobig"})
			continue
		}

		name := safeName(fh.Filename)
		if strings.EqualFold(path.Ext(name), ".zip") || looksLikeZip(data) {
			rep, err := s.lib.ImportZip(r.Context(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				combined.Failed = append(combined.Failed, library.ImportResult{
					Name: name, Key: "why.other", Args: []any{err.Error()}})
				continue
			}
			merge(&combined, rep)
			continue
		}
		if isSheet(name, data) {
			if skip, ok := s.alreadyHave(r.Context(), name, titleOf(name), artist); ok {
				combined.Skipped = append(combined.Skipped, skip)
				continue
			}
		}
		merge(&combined, s.lib.ImportSong(r.Context(), name, asSong(name, artist, verse, data)))
	}

	// How many came in is news, so it is said the way the application says
	// news, in the banner; the report below is the detail.
	p := s.page(r, "transfer.title", importBody{
		Tabs:     s.settingsTabs(r, "import"),
		MaxBytes: s.cfg.MaxUploadBytes,
		Report:   viewOf(say, &combined),
		Artist:   artist,
	})
	if n := len(combined.Added); n == 1 {
		p.Nav.Flash = say.T("import.added.one", n)
	} else if n > 1 {
		p.Nav.Flash = say.T("import.added", n)
	}
	s.render(w, r, http.StatusOK, "import.html", p)
}

// alreadyHave reports a chord sheet the collection already has, and why it is
// being left out.
//
// With no artist given, any song with the title counts: a sheet that does not
// say who it is by is far more likely to be a song already there than a second
// one with the same name, and the reason says whose it is, so that somebody who
// meant another artist knows what to write. With an artist, only that artist's
// song counts.
func (s *Server) alreadyHave(ctx context.Context, name, title, artist string) (library.ImportResult, bool) {
	same, err := s.lib.WithTitle(ctx, title)
	if err != nil || len(same) == 0 {
		return library.ImportResult{}, false
	}
	skip := library.ImportResult{Name: name, Title: title, Slug: same[0].Slug, Key: "why.already"}
	if artist == "" {
		if who := artistsOf(same); len(who) > 0 {
			skip.Key, skip.Args = "why.sametitle", []any{strings.Join(who, ", ")}
		}
		return skip, true
	}
	for _, rec := range same {
		if index.Slugify(rec.Artist) == index.Slugify(artist) {
			skip.Slug = rec.Slug
			return skip, true
		}
	}
	return library.ImportResult{}, false
}

// artistsOf lists the artists of some songs, each once, most songs first.
func artistsOf(recs []index.Record) []string {
	count := map[string]int{}
	var order []string
	for _, rec := range recs {
		if rec.Artist == "" {
			continue
		}
		if count[rec.Artist] == 0 {
			order = append(order, rec.Artist)
		}
		count[rec.Artist]++
	}
	sort.SliceStable(order, func(i, j int) bool { return count[order[i]] > count[order[j]] })
	return order
}

// handleImportKnown answers, for the titles of the files somebody has chosen,
// which of them the collection already has and by whom, so that the import page
// can say so and offer the artist before anything is sent. It reads, so it is
// as public as the library it reads from.
func (s *Server) handleImportKnown(w http.ResponseWriter, r *http.Request) {
	var known knownView
	var all []index.Record
	seen := map[string]bool{}
	for _, t := range r.URL.Query()["t"] {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] || len(seen) >= maxKnownTitles {
			continue
		}
		seen[t] = true
		same, err := s.lib.WithTitle(r.Context(), t)
		if err != nil {
			continue
		}
		for _, rec := range same {
			known.Songs = append(known.Songs, knownSong{Title: rec.Title, Artist: rec.Artist, Slug: rec.Slug})
		}
		all = append(all, same...)
	}
	known.Artists = artistsOf(all)
	s.render(w, r, http.StatusOK, "importknown.html", s.page(r, "", known))
}

// maxKnownTitles bounds how many titles one question may ask about. A folder
// of one artist's songs is well under it.
const maxKnownTitles = 500

// knownView is what the import page is told about the files it is holding.
type knownView struct {
	Songs   []knownSong
	Artists []string
}

type knownSong struct {
	Title, Artist, Slug string
}

// isSheet reports whether an upload is a chord sheet rather than a song file.
//
// A file named .song is never read as a sheet, even when it does not start the
// way a song file starts. It was meant to be one, and whoever uploaded it is
// owed the line it went wrong on rather than a guess at what it said.
func isSheet(name string, data []byte) bool {
	return !strings.EqualFold(path.Ext(name), songfs.Ext) && !song.IsFile(data) && utf8.Valid(data)
}

// titleOf is the title a chord sheet takes from the name of its file.
func titleOf(name string) string {
	return strings.TrimSpace(strings.TrimSuffix(name, path.Ext(name)))
}

// asSong turns an uploaded chord sheet into the text of a .song file, with the
// title taken from the file's name and the artist from the form, and passes
// anything else through exactly as it came. verse is what an unnamed stanza
// after a named one is called, in the reader's language.
func asSong(name, artist, verse string, data []byte) []byte {
	if !isSheet(name, data) {
		return data
	}
	p := song.FromSheet(string(data), verse)
	p.Title, p.Artist = titleOf(name), artist
	return song.Put(p)
}

func merge(into *library.ImportReport, from library.ImportReport) {
	into.Added = append(into.Added, from.Added...)
	into.Skipped = append(into.Skipped, from.Skipped...)
	into.Failed = append(into.Failed, from.Failed...)
}

// looksLikeZip checks the magic bytes, because a browser's idea of a filename is
// not evidence of anything.
func looksLikeZip(data []byte) bool {
	return len(data) > 4 && data[0] == 'P' && data[1] == 'K' &&
		(data[2] == 3 || data[2] == 5 || data[2] == 7)
}

// safeName reduces an uploaded filename to something safe to show in a report.
//
// It is never used as a path. It exists only so the report can say which upload
// a problem came from, and it is stripped of anything that could break a page or
// a log line.
func safeName(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		if b.Len() > 120 {
			break
		}
	}
	if b.Len() == 0 {
		return "(?)"
	}
	return b.String()
}

// handleExport streams the collection as a zip archive.
//
// Backing up is meant to be copying a directory, and this is the same thing
// through a browser. There is nothing proprietary to undo when restoring: the
// archive holds the .song files exactly as they are on disk.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	q := index.ParseQuery(r.URL.Query().Get("q"))
	if v := r.URL.Query().Get("genre"); v != "" {
		q.Genre = v
	}
	if v := r.URL.Query().Get("artist"); v != "" {
		q.Artist = v
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(s.lib.ExportName()))
	w.Header().Set("Cache-Control", "no-store")

	// The response is streamed, so a failure part way through cannot be turned
	// into an error page: the status is already sent. It is logged instead, and
	// the reader gets a short archive they can see is short.
	if _, err := s.lib.ExportZip(r.Context(), w, q); err != nil {
		s.log.Error("the export failed part way through", "error", err)
	}
}
