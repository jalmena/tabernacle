package library

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/songfs"
)

// Import limits. An archive that exceeds any of them is hostile or a mistake,
// and either way is refused rather than absorbed.
const (
	maxZipEntries      = 5000
	maxZipTotalBytes   = 200 << 20 // 200 MiB decompressed
	maxImportSongBytes = 1 << 20   // one .song file
)

// ImportResult is what happened to one file in an import.
type ImportResult struct {
	Name   string // the name inside the archive, for the report only
	Title  string // the parsed title, when it parsed
	Slug   string // where it ended up
	Reason string // why it was skipped or rejected
	Line   int    // the line of a syntax error, when there is one
}

// ImportReport summarises an import.
//
// Failures are reported per file rather than rolled back. An import of five
// hundred songs that stumbles on the four hundred and ninety-ninth must not
// discard the other four hundred and ninety-eight.
type ImportReport struct {
	Added   []ImportResult
	Skipped []ImportResult
	Failed  []ImportResult
}

// Total is how many files were considered.
func (r ImportReport) Total() int { return len(r.Added) + len(r.Skipped) + len(r.Failed) }

// ImportSong takes a single uploaded file.
func (l *Library) ImportSong(ctx context.Context, name string, data []byte) ImportReport {
	var rep ImportReport
	l.importOne(ctx, &rep, name, data)
	if _, err := l.Sync(ctx); err != nil && !errors.Is(err, ErrSyncBusy) {
		l.log.Error("could not reindex after an import", "error", err)
	}
	return rep
}

// ImportZip takes an archive of .song files.
//
// The single most important line in this function is the one that derives the
// destination from the *parsed* title and artist rather than from the archive
// entry's name. The name is used only in the report. That removes the archive's
// paths from the write path altogether, which is a stronger guarantee than
// sanitising them: there is nothing left to sanitise.
func (l *Library) ImportZip(ctx context.Context, r io.ReaderAt, size int64) (ImportReport, error) {
	var rep ImportReport

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return rep, fmt.Errorf("this does not look like a zip file: %w", err)
	}
	if len(zr.File) > maxZipEntries {
		return rep, fmt.Errorf("the archive holds %d entries, more than the limit of %d",
			len(zr.File), maxZipEntries)
	}

	var budget int64 = maxZipTotalBytes
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		name := f.Name

		if reason := rejectZipEntry(f); reason != "" {
			rep.Skipped = append(rep.Skipped, ImportResult{Name: name, Reason: reason})
			continue
		}
		// The declared size is attacker-controlled and may lie, so it is used
		// only as a cheap first filter; the real limit is enforced while reading.
		if f.UncompressedSize64 > maxImportSongBytes {
			rep.Skipped = append(rep.Skipped, ImportResult{
				Name: name, Reason: "el archivo es demasiado grande"})
			continue
		}

		data, err := readZipEntry(f, min64(budget, maxImportSongBytes))
		if err != nil {
			rep.Failed = append(rep.Failed, ImportResult{Name: name, Reason: err.Error()})
			continue
		}
		budget -= int64(len(data))
		if budget <= 0 {
			rep.Failed = append(rep.Failed, ImportResult{
				Name: name, Reason: "se ha alcanzado el límite total de descompresión"})
			break
		}
		l.importOne(ctx, &rep, name, data)
	}

	if _, err := l.Sync(ctx); err != nil && !errors.Is(err, ErrSyncBusy) {
		l.log.Error("could not reindex after an import", "error", err)
	}
	return rep, nil
}

// rejectZipEntry applies the structural checks, returning why an entry is not
// acceptable or "" when it is.
//
// Paths are rejected rather than cleaned. A path that needs cleaning is hostile,
// and quietly repairing it would mean guessing what an attacker meant.
func rejectZipEntry(f *zip.File) string {
	name := f.Name
	switch {
	case strings.HasSuffix(name, "/"):
		return "" // a directory entry; silently ignored below
	case !isPrintableUTF8(name):
		return "el nombre no es texto válido"
	case strings.ContainsRune(name, 0):
		return "el nombre contiene un byte nulo"
	}
	if strings.HasSuffix(name, "/") {
		return "es un directorio"
	}

	slash := filepath.ToSlash(name)
	if slash == "" || strings.HasPrefix(slash, "/") || !fs.ValidPath(strings.TrimPrefix(slash, "./")) {
		return "la ruta sale del archivo"
	}
	if len(slash) > 1 && slash[1] == ':' {
		return "la ruta tiene letra de unidad"
	}
	for _, seg := range strings.Split(slash, "/") {
		if seg == ".." {
			return "la ruta sale del archivo"
		}
	}
	if strings.Contains(name, `\`) {
		return "la ruta usa barras invertidas"
	}

	// Only regular files. A symlink entry is the classic way an archive tries to
	// write outside where it is unpacked, and the archive's mode bits are never
	// honoured in any case.
	mode := f.Mode()
	if mode&fs.ModeSymlink != 0 {
		return "es un enlace simbólico"
	}
	if !mode.IsRegular() {
		return "no es un archivo normal"
	}
	if mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return "tiene permisos especiales"
	}
	if !strings.EqualFold(path.Ext(slash), songfs.Ext) {
		return "no es un archivo .song"
	}
	if base := path.Base(slash); base == songfs.Ext {
		return "no tiene nombre"
	}
	return ""
}

// readZipEntry reads one entry, refusing to exceed the limit whatever its header
// claimed. This is the defence against a small archive that decompresses to
// something enormous.
func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("no se ha podido leer: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("no se ha podido leer: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("el archivo es más grande de lo que declaraba")
	}
	return data, nil
}

// importOne validates and stores one file's worth of bytes.
func (l *Library) importOne(ctx context.Context, rep *ImportReport, name string, data []byte) {
	if int64(len(data)) > maxImportSongBytes {
		rep.Skipped = append(rep.Skipped, ImportResult{
			Name: name, Reason: "el archivo es demasiado grande"})
		return
	}
	// Nothing is written until it parses. Imported files are untrusted input,
	// and a collection of files that will not load is worse than a rejection.
	normalized := normalizeText(string(data))
	s, err := song.Parse(normalized)
	if err != nil {
		res := ImportResult{Name: name, Reason: err.Error()}
		var se *song.SyntaxError
		if errors.As(err, &se) {
			res.Reason = se.Msg
			res.Line = se.Line
		}
		rep.Failed = append(rep.Failed, res)
		return
	}
	canonical, err := song.Marshal(s)
	if err != nil {
		rep.Failed = append(rep.Failed, ImportResult{Name: name, Reason: err.Error()})
		return
	}

	// Restoring a backup over an existing collection must not duplicate it.
	// Both forms are checked: the bytes as uploaded, which match a file written
	// by hand, and the canonical form, which matches one this application wrote.
	for _, candidate := range [][]byte{normalized, canonical} {
		existing, found, err := l.idx.ByHash(ctx, hashBytes(candidate))
		if err != nil {
			rep.Failed = append(rep.Failed, ImportResult{Name: name, Reason: err.Error()})
			return
		}
		if found {
			rep.Skipped = append(rep.Skipped, ImportResult{
				Name:   name,
				Title:  s.Title,
				Slug:   existing.Slug,
				Reason: "ya está en la colección",
			})
			return
		}
	}

	rel, err := l.fs.Create(s.Artist, s.Title, canonical)
	if err != nil {
		rep.Failed = append(rep.Failed, ImportResult{
			Name: name, Title: s.Title, Reason: err.Error()})
		return
	}
	rep.Added = append(rep.Added, ImportResult{
		Name:  name,
		Title: s.Title,
		Slug:  index.SlugForPath(rel),
	})
}

// ExportZip streams the collection as a zip archive.
//
// Entry names come from the indexed paths, which are already known to be inside
// the collection, so the archive mirrors the directory exactly. Restoring is then
// unzipping it — there is nothing proprietary to undo.
func (l *Library) ExportZip(ctx context.Context, w io.Writer, q index.Query) (int, error) {
	q.Limit = 200
	q.Offset = 0

	zw := zip.NewWriter(w)
	written := 0
	stamp := l.now()

	for {
		hits, total, err := l.idx.List(ctx, q)
		if err != nil {
			return written, err
		}
		for _, h := range hits {
			if err := ctx.Err(); err != nil {
				return written, err
			}
			data, _, err := l.fs.ReadRaw(h.RelPath)
			if err != nil {
				// A file that vanished between the query and the read is not a
				// reason to abandon the whole export.
				l.log.Warn("skipped a song while exporting", "path", h.RelPath, "error", err)
				continue
			}
			hdr := &zip.FileHeader{
				Name:     "songs/" + h.RelPath,
				Method:   zip.Deflate,
				Modified: h.ModTime,
			}
			if hdr.Modified.IsZero() {
				hdr.Modified = stamp
			}
			hdr.SetMode(0o644)
			f, err := zw.CreateHeader(hdr)
			if err != nil {
				return written, err
			}
			if _, err := f.Write(data); err != nil {
				return written, err
			}
			written++
		}
		q.Offset += len(hits)
		if len(hits) == 0 || q.Offset >= total {
			break
		}
	}
	return written, zw.Close()
}

// ExportName is the filename to offer for a backup.
func (l *Library) ExportName() string {
	return "tabernacle-" + l.now().Format("2006-01-02") + ".zip"
}

// hashBytes is the same fingerprint the index stores for a file, so an import
// can tell whether it is looking at a song that is already there.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// isPrintableUTF8 reports whether a name is text at all. A name that is not is
// either corrupt or an attempt to smuggle something past a later check.
func isPrintableUTF8(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
