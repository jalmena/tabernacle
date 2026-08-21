package web

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/library"
)

// maxUploadMemory is how much of a multipart form is held in memory before the
// rest spills to a temporary file.
const maxUploadMemory = 8 << 20

func (s *Server) handleImportForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "import.html", s.page(r, "Importar", importBody{
		MaxBytes: s.cfg.MaxUploadBytes,
	}))
}

// importBody is what the import page receives, before and after an upload.
type importBody struct {
	MaxBytes int64
	Report   *library.ImportReport
	Problem  string
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
			s.page(r, "Importar", importBody{
				MaxBytes: s.cfg.MaxUploadBytes,
				Problem: "El envío es demasiado grande o no se ha podido leer. " +
					"Prueba con menos archivos o con un zip más pequeño.",
			}))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	var combined library.ImportReport
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		s.render(w, r, http.StatusBadRequest, "import.html", s.page(r, "Importar", importBody{
			MaxBytes: s.cfg.MaxUploadBytes,
			Problem:  "No se ha seleccionado ningún archivo.",
		}))
		return
	}

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			combined.Failed = append(combined.Failed, library.ImportResult{
				Name: safeName(fh.Filename), Reason: "no se ha podido leer"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes+1))
		f.Close()
		if err != nil || int64(len(data)) > s.cfg.MaxUploadBytes {
			combined.Failed = append(combined.Failed, library.ImportResult{
				Name: safeName(fh.Filename), Reason: "el archivo es demasiado grande"})
			continue
		}

		name := safeName(fh.Filename)
		if strings.EqualFold(path.Ext(name), ".zip") || looksLikeZip(data) {
			rep, err := s.lib.ImportZip(r.Context(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				combined.Failed = append(combined.Failed, library.ImportResult{
					Name: name, Reason: err.Error()})
				continue
			}
			merge(&combined, rep)
			continue
		}
		merge(&combined, s.lib.ImportSong(r.Context(), name, data))
	}

	s.render(w, r, http.StatusOK, "import.html", s.page(r, "Importar", importBody{
		MaxBytes: s.cfg.MaxUploadBytes,
		Report:   &combined,
	}))
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
		return "(sin nombre)"
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
