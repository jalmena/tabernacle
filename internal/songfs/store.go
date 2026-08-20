// Package songfs is the only part of Tabernacle that touches the filesystem.
//
// It owns the collection: a directory of .song files that is the sole source of
// truth for song content. Everything else in the application treats that
// directory as authoritative and the database as a rebuildable index.
//
// Confinement is enforced by os.Root, which refuses to escape the collection at
// every syscall rather than after a check that a symlink could invalidate in
// between. Path validation in CheckRel sits in front of that as a cheap,
// explicit layer with clear error messages, not as the actual defence.
package songfs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jalmena/tabernacle/internal/song"
)

var (
	// ErrTooLarge reports a file bigger than the configured per-song limit.
	ErrTooLarge = errors.New("song file is too large")
	// ErrConflict reports that the file changed since the caller read it.
	ErrConflict = errors.New("song file changed since it was read")
	// ErrExists reports a destination that is already occupied.
	ErrExists = errors.New("song file already exists")
)

// Store is a confined handle on the collection.
type Store struct {
	songs    *os.Root
	trash    *os.Root
	maxBytes int64
}

// Open creates the directories if needed and opens confined handles on them.
//
// The trash directory is a second root rather than a subdirectory of the
// collection, so that deleted songs cannot be re-indexed and do not appear
// while browsing.
func Open(songsDir, trashDir string, maxBytes int64) (*Store, error) {
	if maxBytes <= 0 {
		return nil, errors.New("songfs: maxBytes must be greater than zero")
	}
	if err := os.MkdirAll(songsDir, 0o755); err != nil {
		return nil, fmt.Errorf("songfs: create %s: %w", songsDir, err)
	}
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return nil, fmt.Errorf("songfs: create %s: %w", trashDir, err)
	}
	songs, err := os.OpenRoot(songsDir)
	if err != nil {
		return nil, fmt.Errorf("songfs: open %s: %w", songsDir, err)
	}
	trash, err := os.OpenRoot(trashDir)
	if err != nil {
		songs.Close()
		return nil, fmt.Errorf("songfs: open %s: %w", trashDir, err)
	}
	return &Store{songs: songs, trash: trash, maxBytes: maxBytes}, nil
}

// Close releases the directory handles.
func (s *Store) Close() error {
	err := s.songs.Close()
	if terr := s.trash.Close(); err == nil {
		err = terr
	}
	return err
}

// Entry is one file in the collection.
//
// Err is set when the file could not be read or parsed, in which case Song is
// zero. A broken file is still an Entry: it has to remain visible in the
// interface so it can be repaired, because a song that vanished because of a
// typo looks exactly like a song that was lost.
//
// Raw carries the file's bytes only for Read and ReadRaw. Scan leaves it nil,
// so walking a large collection does not hold every song in memory at once.
type Entry struct {
	RelPath string
	Size    int64
	ModTime time.Time
	Hash    string // sha256 of the raw bytes, hex encoded
	Raw     []byte
	Song    song.Song
	Err     error
}

// Scan walks the collection and calls yield once per song file, in directory
// order.
//
// A file that will not parse is reported with Err set rather than aborting the
// walk: one bad file must never hide the rest of a collection. Symlinks,
// dotfiles and anything that is not a regular .song file are skipped silently.
//
// If yield returns an error the walk stops and returns it, which is how a
// caller cancels a scan.
func (s *Store) Scan(yield func(Entry) error) error {
	fsys := s.songs.FS()
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory should not abort the whole scan; skip it
			// and let the rest of the collection be indexed.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == "." {
			return nil
		}
		base := path.Base(p)

		// Symlinks are never followed and never indexed. os.Root already
		// prevents one escaping the collection, but a symlink inside it would
		// still index the same song twice, and a loop of them would hang the
		// walk.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasPrefix(base, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if strings.Count(p, "/")+1 >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		// Skip devices, sockets and FIFOs: os.Root deliberately does not.
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.EqualFold(path.Ext(base), Ext) || base == Ext {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return yield(Entry{RelPath: p, Err: err})
		}
		e := Entry{RelPath: p, Size: info.Size(), ModTime: info.ModTime()}
		if info.Size() > s.maxBytes {
			e.Err = fmt.Errorf("%w: %d bytes, limit is %d", ErrTooLarge, info.Size(), s.maxBytes)
			return yield(e)
		}

		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			e.Err = err
			return yield(e)
		}
		e.Hash = hashOf(data)
		if sg, perr := song.Parse(data); perr != nil {
			e.Err = perr
		} else {
			e.Song = sg
		}
		return yield(e)
	})
}

// Read returns one song, parsed, with its bytes.
func (s *Store) Read(rel string) (Entry, error) {
	data, hash, info, err := s.readRaw(rel)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{
		RelPath: rel,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Hash:    hash,
		Raw:     data,
	}
	if sg, perr := song.Parse(data); perr != nil {
		e.Err = perr
	} else {
		e.Song = sg
	}
	return e, nil
}

// ReadRaw returns a song's bytes and their hash without parsing.
//
// The raw editor needs this: a file that will not parse is exactly the file the
// user is trying to fix, so it has to be readable in its broken state.
func (s *Store) ReadRaw(rel string) (data []byte, hash string, err error) {
	data, hash, _, err = s.readRaw(rel)
	return data, hash, err
}

func (s *Store) readRaw(rel string) ([]byte, string, os.FileInfo, error) {
	if err := CheckRel(rel); err != nil {
		return nil, "", nil, err
	}
	f, err := s.songs.Open(rel)
	if err != nil {
		return nil, "", nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, "", nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, "", nil, fmt.Errorf("%s is not a regular file: %w", rel, ErrNotLocal)
	}
	if info.Size() > s.maxBytes {
		return nil, "", nil, fmt.Errorf("%w: %d bytes, limit is %d", ErrTooLarge, info.Size(), s.maxBytes)
	}
	// Read one byte past the limit so a file that grows between Stat and Read
	// is caught rather than silently truncated.
	data, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		return nil, "", nil, err
	}
	if int64(len(data)) > s.maxBytes {
		return nil, "", nil, fmt.Errorf("%w: limit is %d", ErrTooLarge, s.maxBytes)
	}
	return data, hashOf(data), info, nil
}

// Exists reports whether a song file is present.
func (s *Store) Exists(rel string) (bool, error) {
	if err := CheckRel(rel); err != nil {
		return false, err
	}
	_, err := s.songs.Lstat(rel)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// WriteRaw replaces a song's bytes.
//
// expectHash implements optimistic concurrency: pass the hash the caller last
// read and the write fails with ErrConflict if the file has changed since. With
// a periodic rescan and an edit form left open on a phone, a lost update is not
// hypothetical. Pass "" to write unconditionally.
//
// When the bytes are already identical nothing is written at all, and the file's
// modification time is left alone. That is what makes opening a song free of
// side effects: no spurious diff in a git-tracked collection, and no rescan
// churn.
func (s *Store) WriteRaw(rel string, data []byte, expectHash string) (string, error) {
	if err := CheckRel(rel); err != nil {
		return "", err
	}
	if int64(len(data)) > s.maxBytes {
		return "", fmt.Errorf("%w: %d bytes, limit is %d", ErrTooLarge, len(data), s.maxBytes)
	}

	current, currentHash, _, err := s.readRaw(rel)
	switch {
	case err == nil:
		if expectHash != "" && currentHash != expectHash {
			return "", fmt.Errorf("%w: expected %s, found %s", ErrConflict, short(expectHash), short(currentHash))
		}
		if string(current) == string(data) {
			return currentHash, nil // nothing changed; leave the file untouched
		}
	case errors.Is(err, fs.ErrNotExist):
		if expectHash != "" {
			return "", fmt.Errorf("%w: the file no longer exists", ErrConflict)
		}
	default:
		return "", err
	}

	if err := s.atomicWrite(rel, data); err != nil {
		return "", err
	}
	return hashOf(data), nil
}

// Create writes a new song at a path derived from its own metadata, choosing a
// free filename rather than overwriting anything.
//
// The destination comes from RelPathFor, never from caller-supplied text, which
// is what keeps an import safe: the name inside a zip entry is only ever used
// in the report.
func (s *Store) Create(artist, title string, data []byte) (string, error) {
	if int64(len(data)) > s.maxBytes {
		return "", fmt.Errorf("%w: %d bytes, limit is %d", ErrTooLarge, len(data), s.maxBytes)
	}
	base := RelPathFor(artist, title)
	if err := CheckRel(base); err != nil {
		return "", err
	}
	rel := base
	for n := 2; ; n++ {
		err := s.createExclusive(rel, data)
		if err == nil {
			return rel, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		if n > 999 {
			return "", fmt.Errorf("%w: %s and 999 alternatives", ErrExists, base)
		}
		rel = withSuffix(base, n)
		if err := CheckRel(rel); err != nil {
			return "", err
		}
	}
}

// Move renames a song within the collection.
func (s *Store) Move(oldRel, newRel string) error {
	if err := CheckRel(oldRel); err != nil {
		return err
	}
	if err := CheckRel(newRel); err != nil {
		return err
	}
	if oldRel == newRel {
		return nil
	}
	if ok, err := s.Exists(newRel); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("%w: %s", ErrExists, newRel)
	}
	if dir := path.Dir(newRel); dir != "." {
		if err := s.songs.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := s.songs.Rename(oldRel, newRel); err != nil {
		return err
	}
	s.pruneEmptyDirs(path.Dir(oldRel))
	return nil
}

// Trash moves a song out of the collection instead of deleting it.
//
// Undoing a mistaken delete should be a file manager operation, not a restore
// from backup. The name is prefixed with a timestamp so repeated deletes of the
// same song do not collide.
func (s *Store) Trash(rel string) (string, error) {
	data, _, _, err := s.readRaw(rel)
	if err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	name := stamp + "-" + strings.ReplaceAll(rel, "/", "_")

	// Copied rather than renamed: the trash is a separate confined root, and it
	// may well be on a different filesystem.
	dest := name
	for n := 2; ; n++ {
		err := writeExclusive(s.trash, dest, data)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		if n > 999 {
			return "", fmt.Errorf("%w: %s", ErrExists, name)
		}
		dest = withSuffix(name, n)
	}
	if err := s.songs.Remove(rel); err != nil {
		return "", err
	}
	s.pruneEmptyDirs(path.Dir(rel))
	return dest, nil
}

// atomicWrite writes data to rel by way of a temporary file in the same
// directory, so a reader never observes a partial song and a crash mid-write
// cannot corrupt the previous version.
func (s *Store) atomicWrite(rel string, data []byte) error {
	dir := path.Dir(rel)
	if dir != "." {
		if err := s.songs.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := s.createTemp(dir)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.songs.Remove(tmp)
		}
	}()

	f, err := s.songs.OpenFile(tmp, os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	// Durability before visibility: fsync the contents, then rename, then fsync
	// the directory so the rename itself survives a power cut.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := s.songs.Rename(tmp, rel); err != nil {
		return err
	}
	committed = true
	s.syncDir(dir)
	return nil
}

func (s *Store) createExclusive(rel string, data []byte) error {
	if dir := path.Dir(rel); dir != "." {
		if err := s.songs.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := writeExclusive(s.songs, rel, data); err != nil {
		return err
	}
	s.syncDir(path.Dir(rel))
	return nil
}

// writeExclusive creates a file, failing if it already exists.
//
// O_EXCL rather than a Stat-then-create is what makes choosing a free filename
// race-free: two concurrent imports of the same song cannot both decide the
// same name is available and have one silently overwrite the other.
func writeExclusive(root *os.Root, rel string, data []byte) error {
	f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = root.Remove(rel)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = root.Remove(rel)
		return err
	}
	return f.Close()
}

// createTemp makes an empty temporary file in dir and returns its path.
func (s *Store) createTemp(dir string) (string, error) {
	var buf [8]byte
	for i := 0; i < 100; i++ {
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		name := ".tabernacle-" + hex.EncodeToString(buf[:]) + ".tmp"
		if dir != "." {
			name = dir + "/" + name
		}
		f, err := s.songs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			f.Close()
			return name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("songfs: could not create a temporary file")
}

// syncDir flushes a directory entry so a rename survives a power failure.
// Failure is not fatal: the data is already written, and some filesystems do
// not support it.
func (s *Store) syncDir(dir string) {
	if dir == "" {
		dir = "."
	}
	f, err := s.songs.Open(dir)
	if err != nil {
		return
	}
	_ = f.Sync()
	_ = f.Close()
}

// pruneEmptyDirs removes directories left empty by a move or a delete, so that
// renaming the last song of an artist does not leave an empty folder behind.
// It stops at the first non-empty directory and never touches the root.
func (s *Store) pruneEmptyDirs(dir string) {
	for dir != "." && dir != "/" && dir != "" {
		entries, err := fs.ReadDir(s.songs.FS(), dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := s.songs.Remove(dir); err != nil {
			return
		}
		dir = path.Dir(dir)
	}
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
