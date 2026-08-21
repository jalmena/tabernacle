package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/songfs"
)

// ErrReadOnly reports that the collection may not be changed.
var ErrReadOnly = errors.New("the collection is read-only")

// ErrConflict reports that the file changed since the caller read it.
var ErrConflict = songfs.ErrConflict

// SyntaxError is re-exported so the HTTP layer can report the line to fix
// without importing the song package for the sole purpose of a type assertion.
type SyntaxError = song.SyntaxError

// CreateFromText saves a pasted song.
//
// The text is parsed and validated before anything touches disk, and the
// destination path is derived from the parsed title and artist rather than from
// anything the caller supplied. That is what keeps every write inside the
// collection by construction.
func (l *Library) CreateFromText(ctx context.Context, text string) (slug string, err error) {
	data := normalizeText(text)
	s, err := song.Parse(data)
	if err != nil {
		return "", err
	}
	// Re-marshal so what lands on disk is canonical: consistent field order,
	// quoting where a value needs it, and a byte-identical round trip from then
	// on. A pasted file and one written by the application are then the same
	// file.
	canonical, err := song.Marshal(s)
	if err != nil {
		return "", err
	}

	rel, err := l.fs.Create(s.Artist, s.Title, canonical)
	if err != nil {
		return "", err
	}
	return l.slugAfterWrite(ctx, rel)
}

// SaveRaw replaces a song's file with the text the user typed.
//
// expectHash carries the version the editor loaded, so an edit made from another
// device, or by hand on the server, is reported as a conflict rather than
// silently overwritten.
//
// The bytes are stored exactly as typed rather than re-marshalled. Somebody
// editing the raw file is entitled to their own field order and their own
// spacing; rewriting it under them would be rude and would produce a diff they
// did not ask for.
func (l *Library) SaveRaw(ctx context.Context, slug string, text string, expectHash string) (string, error) {
	rec, found, err := l.idx.BySlug(ctx, slug)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNotFound
	}

	data := normalizeText(text)
	if _, err := song.Parse(data); err != nil {
		return "", err
	}
	if _, err := l.fs.WriteRaw(rec.RelPath, data, expectHash); err != nil {
		return "", err
	}
	return l.slugAfterWrite(ctx, rec.RelPath)
}

// CommitTranspose writes a transposition into the file.
//
// This is the deliberate, separate counterpart to transposing for reading: that
// never touches a file, and this always does. The saved reading transposition is
// cleared afterwards, since the file is now in the new key and applying it again
// would move the song twice.
func (l *Library) CommitTranspose(ctx context.Context, slug string, semitones int) error {
	if semitones%12 == 0 {
		return nil // nothing to write
	}
	if err := l.rewrite(ctx, slug, func(s song.Song) (song.Song, error) {
		return s.Transpose(semitones), nil
	}); err != nil {
		return err
	}
	st, err := l.idx.State(ctx, slug)
	if err != nil {
		return err
	}
	st.Transpose = 0
	return l.idx.SetState(ctx, slug, st)
}

// Delete moves a song out of the collection into the trash directory.
func (l *Library) Delete(ctx context.Context, slug string) error {
	rec, found, err := l.idx.BySlug(ctx, slug)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	if _, err := l.fs.Trash(rec.RelPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	// The file is gone, so drop the row now rather than leaving the song
	// listed until the next scan.
	if err := l.idx.DeleteByRelPath(ctx, rec.RelPath); err != nil {
		l.log.Error("deleted a song but could not update the index",
			"path", rec.RelPath, "error", err)
	}
	l.log.Info("song moved to the trash", "path", rec.RelPath)
	return nil
}

// rewrite loads a song, transforms it and writes it back canonically.
//
// Marshal guarantees a byte-identical round trip, so a change of one field
// produces a diff of one field, and songfs skips the write entirely when the
// bytes are unchanged.
func (l *Library) rewrite(ctx context.Context, slug string, fn func(song.Song) (song.Song, error)) error {
	rec, found, err := l.idx.BySlug(ctx, slug)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	entry, err := l.fs.Read(rec.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	if entry.Err != nil {
		// Refusing here is the honest answer: rewriting a file that does not
		// parse would mean discarding whatever the user was in the middle of
		// fixing.
		return fmt.Errorf("this file has a syntax error and must be repaired first: %w", entry.Err)
	}

	out, err := fn(entry.Song)
	if err != nil {
		return err
	}
	data, err := song.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := l.fs.WriteRaw(rec.RelPath, data, entry.Hash); err != nil {
		return err
	}
	_, err = l.slugAfterWrite(ctx, rec.RelPath)
	return err
}

// slugAfterWrite brings the index up to date and reports where the song now
// lives.
//
// A full scan is used rather than indexing the one file, because a slug is only
// unique with respect to every other path: resolving a collision means knowing
// what it collides with. At the scale of a personal collection a scan is a few
// milliseconds, and reusing Sync means there is one code path that decides what
// the index contains rather than two that can disagree.
func (l *Library) slugAfterWrite(ctx context.Context, rel string) (string, error) {
	if _, err := l.Sync(ctx); err != nil && !errors.Is(err, ErrSyncBusy) {
		return "", err
	}
	stamps, err := l.idx.Stamps(ctx)
	if err != nil {
		return "", err
	}
	if st, ok := stamps[rel]; ok {
		return st.Slug, nil
	}
	// A scan was already running and has not reached this file yet. The slug is
	// deterministic from the path, so it can be named without waiting.
	return index.SlugForPath(rel), nil
}

// normalizeText prepares pasted or typed text for the parser.
//
// Line endings are normalised and a final newline is ensured, because a textarea
// submits CRLF and a file with no trailing newline is a nuisance in every tool
// that touches it afterwards.
func normalizeText(text string) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text)
}
