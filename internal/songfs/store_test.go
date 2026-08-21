package songfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
)

const testMax = 1 << 20

// songBytes builds a minimal valid .song file.
func songBytes(title, artist string) []byte {
	return []byte(fmt.Sprintf("---\ntitle: %s\nartist: %s\n---\n\n## Verse 1\n[G]Hello [C]world\n",
		title, artist))
}

// newStore returns a Store over fresh temporary directories, plus the two
// absolute paths so a test can plant hostile files behind the Store's back.
func newStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	base := t.TempDir()
	songs := filepath.Join(base, "songs")
	trash := filepath.Join(base, "trash")
	s, err := Open(songs, trash, testMax)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, songs, trash
}

// write places a file directly on disk, bypassing the Store.
func write(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanAll(t *testing.T, s *Store) []Entry {
	t.Helper()
	var got []Entry
	if err := s.Scan(func(e Entry) error { got = append(got, e); return nil }); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].RelPath < got[j].RelPath })
	return got
}

func relPaths(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.RelPath
	}
	return out
}

func TestScanFindsSongs(t *testing.T) {
	s, songs, _ := newStore(t)
	write(t, songs, "Traditional/Amazing Grace.song", songBytes("Amazing Grace", "Traditional"))
	write(t, songs, "Bill Withers/Ain't No Sunshine.song", songBytes("Ain't No Sunshine", "Bill Withers"))
	write(t, songs, "Loose.song", songBytes("Loose", ""))

	got := scanAll(t, s)
	want := []string{"Bill Withers/Ain't No Sunshine.song", "Loose.song", "Traditional/Amazing Grace.song"}
	if diff := strings.Join(relPaths(got), "|"); diff != strings.Join(want, "|") {
		t.Fatalf("Scan found %v, want %v", relPaths(got), want)
	}
	for _, e := range got {
		if e.Err != nil {
			t.Errorf("%s: unexpected error %v", e.RelPath, e.Err)
		}
		if e.Song.Title == "" {
			t.Errorf("%s: song was not parsed", e.RelPath)
		}
		if len(e.Hash) != 64 {
			t.Errorf("%s: hash = %q, want 64 hex characters", e.RelPath, e.Hash)
		}
		if e.Raw != nil {
			t.Errorf("%s: Scan should not retain the file bytes", e.RelPath)
		}
		if e.Size == 0 {
			t.Errorf("%s: size not recorded", e.RelPath)
		}
	}
}

// A symlink is never followed and never indexed. os.Root already stops one
// escaping the collection; skipping them also prevents indexing a song twice
// and prevents a loop hanging the walk.
func TestScanSkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	s, songs, _ := newStore(t)
	write(t, songs, "Real/Song.song", songBytes("Song", "Real"))

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// A symlink to a file outside the collection.
	must(os.Symlink("/etc/passwd", filepath.Join(songs, "passwd.song")))
	// A symlink to a directory outside the collection.
	must(os.Symlink("/etc", filepath.Join(songs, "escape")))
	// A symlink loop.
	must(os.Symlink(".", filepath.Join(songs, "loop")))
	// A symlink to a real song inside the collection: still skipped, so the
	// song is not indexed under two different paths.
	must(os.Symlink("Real/Song.song", filepath.Join(songs, "alias.song")))

	got := relPaths(scanAll(t, s))
	if len(got) != 1 || got[0] != "Real/Song.song" {
		t.Fatalf("Scan found %v, want only [Real/Song.song]", got)
	}
	for _, e := range scanAll(t, s) {
		if strings.Contains(string(e.Song.Notes), "root:") {
			t.Fatal("scan leaked the contents of /etc/passwd")
		}
	}
}

func TestReadRefusesSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	s, songs, _ := newStore(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(songs, "passwd.song")); err != nil {
		t.Fatal(err)
	}
	data, _, err := s.ReadRaw("passwd.song")
	if err == nil {
		t.Fatalf("ReadRaw followed a symlink out of the collection and returned %d bytes", len(data))
	}
	if strings.Contains(string(data), "root:") {
		t.Fatal("ReadRaw leaked /etc/passwd")
	}
}

func TestScanSkipsJunk(t *testing.T) {
	s, songs, _ := newStore(t)
	write(t, songs, "Real.song", songBytes("Real", ""))
	write(t, songs, "notes.txt", []byte("not a song"))
	write(t, songs, "README.md", []byte("# hello"))
	write(t, songs, ".hidden.song", songBytes("Hidden", ""))
	write(t, songs, ".git/config.song", songBytes("Git", ""))
	write(t, songs, "Artist/.DS_Store", []byte("junk"))
	// A leftover temporary file from an interrupted write must not be indexed.
	write(t, songs, ".tabernacle-deadbeef.tmp", songBytes("Temp", ""))

	got := relPaths(scanAll(t, s))
	if len(got) != 1 || got[0] != "Real.song" {
		t.Fatalf("Scan found %v, want only [Real.song]", got)
	}
}

func TestScanSkipsNonRegularFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("mkfifo is Linux-specific here")
	}
	s, songs, _ := newStore(t)
	write(t, songs, "Real.song", songBytes("Real", ""))
	// os.Root deliberately does not refuse device or FIFO files, so Scan must.
	// Reading a FIFO would block the scan forever.
	if err := syscall.Mkfifo(filepath.Join(songs, "pipe.song"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := relPaths(scanAll(t, s))
	if len(got) != 1 || got[0] != "Real.song" {
		t.Fatalf("Scan found %v, want only [Real.song]", got)
	}
}

// A file that will not parse stays visible with its error, because a song that
// disappeared because of a typo looks exactly like a song that was lost.
func TestScanReportsBrokenFiles(t *testing.T) {
	s, songs, _ := newStore(t)
	write(t, songs, "Good.song", songBytes("Good", ""))
	write(t, songs, "Broken.song", []byte("---\ntitel: typo\n---\n"))

	got := scanAll(t, s)
	if len(got) != 2 {
		t.Fatalf("Scan found %v, want both files", relPaths(got))
	}
	broken, good := got[0], got[1]
	if broken.RelPath != "Broken.song" {
		broken, good = good, broken
	}
	if broken.Err == nil {
		t.Error("the broken file was reported without an error")
	}
	if broken.Hash == "" {
		t.Error("the broken file should still be hashed, so a repair is noticed")
	}
	if good.Err != nil {
		t.Errorf("the good file reported %v", good.Err)
	}
}

func TestScanReportsOversizeFiles(t *testing.T) {
	base := t.TempDir()
	songs, trash := filepath.Join(base, "songs"), filepath.Join(base, "trash")
	s, err := Open(songs, trash, 128)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	write(t, songs, "Huge.song", append(songBytes("Huge", ""), strings.Repeat("x", 500)...))

	got := scanAll(t, s)
	if len(got) != 1 {
		t.Fatalf("Scan found %v, want the oversize file reported", relPaths(got))
	}
	if !errors.Is(got[0].Err, ErrTooLarge) {
		t.Errorf("error = %v, want ErrTooLarge", got[0].Err)
	}
}

func TestScanRespectsDepthLimit(t *testing.T) {
	s, songs, _ := newStore(t)
	write(t, songs, "a/b/c/d/e/f/ok.song", songBytes("OK", ""))
	write(t, songs, "a/b/c/d/e/f/g/h/deep.song", songBytes("Deep", ""))

	for _, p := range relPaths(scanAll(t, s)) {
		if strings.Count(p, "/")+1 > maxDepth {
			t.Errorf("Scan descended past the depth limit: %q", p)
		}
	}
}

func TestScanStopsOnYieldError(t *testing.T) {
	s, songs, _ := newStore(t)
	for i := range 5 {
		write(t, songs, fmt.Sprintf("Song%d.song", i), songBytes("S", ""))
	}
	sentinel := errors.New("cancelled")
	n := 0
	err := s.Scan(func(Entry) error {
		n++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Scan error = %v, want the yield error", err)
	}
	if n != 1 {
		t.Errorf("yield called %d times, want 1", n)
	}
}

func TestCreateReadRoundTrip(t *testing.T) {
	s, _, _ := newStore(t)
	data := songBytes("Amazing Grace", "Traditional")
	rel, err := s.Create("Traditional", "Amazing Grace", data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rel != "Traditional/Amazing Grace.song" {
		t.Errorf("Create put the song at %q", rel)
	}
	e, err := s.Read(rel)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(e.Raw) != string(data) {
		t.Error("Read returned different bytes than Create wrote")
	}
	if e.Song.Title != "Amazing Grace" {
		t.Errorf("title = %q", e.Song.Title)
	}
	if e.Err != nil {
		t.Errorf("parse error: %v", e.Err)
	}
}

// Two songs that sanitise to the same filename must both survive.
func TestCreateChoosesAFreeName(t *testing.T) {
	s, _, _ := newStore(t)
	var got []string
	for range 3 {
		rel, err := s.Create("Traditional", "Amazing Grace", songBytes("Amazing Grace", "Traditional"))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got = append(got, rel)
	}
	want := []string{
		"Traditional/Amazing Grace.song",
		"Traditional/Amazing Grace (2).song",
		"Traditional/Amazing Grace (3).song",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Create #%d = %q, want %q", i+1, got[i], want[i])
		}
	}
	if n := len(scanAll(t, s)); n != 3 {
		t.Errorf("collection holds %d songs, want 3: a Create overwrote another", n)
	}
}

// Opening a song must have no side effects on disk: no new mtime, no diff in a
// git-tracked collection, nothing for the rescan to notice.
func TestWriteRawUnchangedIsANoOp(t *testing.T) {
	s, songs, _ := newStore(t)
	data := songBytes("Song", "Artist")
	rel, err := s.Create("Artist", "Song", data)
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(songs, filepath.FromSlash(rel))
	before, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Read(rel)

	hash, err := s.WriteRaw(rel, data, e.Hash)
	if err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	if hash != e.Hash {
		t.Errorf("hash changed from %s to %s without a content change", e.Hash, hash)
	}
	after, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("modification time changed from %v to %v; the file was rewritten needlessly",
			before.ModTime(), after.ModTime())
	}
}

func TestWriteRawDetectsConflict(t *testing.T) {
	s, songs, _ := newStore(t)
	rel, err := s.Create("Artist", "Song", songBytes("Song", "Artist"))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Read(rel)

	// Somebody else edits the file: another device, or a text editor on the box.
	write(t, songs, rel, songBytes("Song edited elsewhere", "Artist"))

	_, err = s.WriteRaw(rel, songBytes("My version", "Artist"), e.Hash)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("WriteRaw error = %v, want ErrConflict", err)
	}
	// The other edit must survive.
	cur, _, _ := s.ReadRaw(rel)
	if !strings.Contains(string(cur), "edited elsewhere") {
		t.Error("the conflicting write clobbered the other edit")
	}
	// An empty expectHash means "write regardless".
	if _, err := s.WriteRaw(rel, songBytes("Forced", "Artist"), ""); err != nil {
		t.Errorf("unconditional WriteRaw: %v", err)
	}
}

func TestWriteRawLeavesNoTemporaryFiles(t *testing.T) {
	s, songs, _ := newStore(t)
	rel, err := s.Create("Artist", "Song", songBytes("Song", "Artist"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if _, err := s.WriteRaw(rel, songBytes(fmt.Sprintf("Song %d", i), "Artist"), ""); err != nil {
			t.Fatal(err)
		}
	}
	err = filepath.WalkDir(songs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.Contains(d.Name(), ".tmp") {
			t.Errorf("left a temporary file behind: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrashKeepsTheFile(t *testing.T) {
	s, songs, trash := newStore(t)
	rel, err := s.Create("Artist", "Song", songBytes("Song", "Artist"))
	if err != nil {
		t.Fatal(err)
	}
	dest, err := s.Trash(rel)
	if err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(songs, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Error("the song is still in the collection")
	}
	kept, err := os.ReadFile(filepath.Join(trash, dest))
	if err != nil {
		t.Fatalf("the trashed file is not readable: %v", err)
	}
	if !strings.Contains(string(kept), "title: Song") {
		t.Error("the trashed file does not hold the song")
	}
	if !strings.Contains(dest, "Artist_Song.song") {
		t.Errorf("trashed name %q should record where the song came from", dest)
	}
	if _, err := os.Stat(filepath.Join(songs, "Artist")); !os.IsNotExist(err) {
		t.Error("the empty artist directory was left behind")
	}
	// Deleting the same song twice must not collide.
	if _, err := s.Create("Artist", "Song", songBytes("Song", "Artist")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Trash("Artist/Song.song"); err != nil {
		t.Errorf("second Trash: %v", err)
	}
	entries, _ := os.ReadDir(trash)
	if len(entries) != 2 {
		t.Errorf("trash holds %d files, want 2", len(entries))
	}
}

// Every method that takes a path must refuse a hostile one, and must do so
// without touching the filesystem outside the collection.
func TestStoreRefusesHostilePaths(t *testing.T) {
	hostile := []string{
		"../escaped.song",
		"../../etc/passwd.song",
		"Artist/../../escaped.song",
		"/etc/passwd.song",
		`..\..\escaped.song`,
		"Artist/So\x00ng.song",
		".",
		"..",
		"",
		"Artist//Song.song",
		"a/b/c/d/e/f/g/h/i.song",
		"Artist/Song.txt",
		strings.Repeat("a", 9000) + ".song",
	}
	for _, rel := range hostile {
		t.Run(rel, func(t *testing.T) {
			s, songs, _ := newStore(t)
			outside := filepath.Join(filepath.Dir(songs), "escaped.song")

			if _, err := s.Read(rel); err == nil {
				t.Errorf("Read(%q) succeeded", rel)
			}
			if _, _, err := s.ReadRaw(rel); err == nil {
				t.Errorf("ReadRaw(%q) succeeded", rel)
			}
			if _, err := s.WriteRaw(rel, songBytes("Evil", ""), ""); err == nil {
				t.Errorf("WriteRaw(%q) succeeded", rel)
			}
			if _, err := s.Trash(rel); err == nil {
				t.Errorf("Trash(%q) succeeded", rel)
			}
			if _, err := os.Stat(outside); err == nil {
				t.Fatalf("a file was created outside the collection at %s", outside)
			}
		})
	}
}

func TestWriteRawRefusesOversizeContent(t *testing.T) {
	base := t.TempDir()
	s, err := Open(filepath.Join(base, "songs"), filepath.Join(base, "trash"), 256)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.WriteRaw("Big.song", []byte(strings.Repeat("x", 300)), ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("WriteRaw error = %v, want ErrTooLarge", err)
	}
	if _, err := s.Create("A", "Big", []byte(strings.Repeat("x", 300))); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Create error = %v, want ErrTooLarge", err)
	}
}

func TestOpenRejectsBadArguments(t *testing.T) {
	base := t.TempDir()
	if _, err := Open(filepath.Join(base, "s"), filepath.Join(base, "t"), 0); err == nil {
		t.Error("Open accepted maxBytes of zero")
	}
}

// Paths and Scan must agree about what counts as a song, or slug resolution
// would be computed over a different set of files than the one indexed.
func TestPathsAgreesWithScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	s, songs, _ := newStore(t)
	write(t, songs, "Traditional/Amazing Grace.song", songBytes("Amazing Grace", "Traditional"))
	write(t, songs, "Broken.song", []byte("not a song at all"))
	write(t, songs, "notes.txt", []byte("ignore me"))
	write(t, songs, ".hidden.song", songBytes("Hidden", ""))
	write(t, songs, ".git/x.song", songBytes("Git", ""))
	write(t, songs, "a/b/c/d/e/f/g/h/deep.song", songBytes("Deep", ""))
	if err := os.Symlink("/etc/passwd", filepath.Join(songs, "passwd.song")); err != nil {
		t.Fatal(err)
	}

	paths, err := s.Paths()
	if err != nil {
		t.Fatalf("Paths: %v", err)
	}
	sort.Strings(paths)
	scanned := relPaths(scanAll(t, s))

	if strings.Join(paths, "|") != strings.Join(scanned, "|") {
		t.Errorf("Paths gave %v but Scan gave %v", paths, scanned)
	}
	// A file that will not parse is still a song file: it must be indexed so it
	// can be found and repaired.
	if len(paths) != 2 {
		t.Errorf("Paths = %v, want the good song and the broken one", paths)
	}
}
