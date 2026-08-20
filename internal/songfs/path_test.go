package songfs

import (
	"errors"
	"path"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCheckRelAccepts(t *testing.T) {
	for _, rel := range []string{
		"Amazing Grace.song",
		"Traditional/Amazing Grace.song",
		"Bill Withers/Ain't No Sunshine.song",
		"Joaquín Sabina/Y nos dieron las diez.song",
		"a/b/c/d/e/f/g.song",   // exactly maxDepth
		"Artist/Song (2).song", // the collision suffix must round-trip
		"Sigur Rós/().song",
		"AC-DC/T.N.T..song",
		"x.SONG", // extension compared case-insensitively
	} {
		if err := CheckRel(rel); err != nil {
			t.Errorf("CheckRel(%q) = %v, want nil", rel, err)
		}
	}
}

// The traversal table. Everything here must be refused, and refused as
// ErrNotLocal so a caller can tell an attack from an I/O failure.
func TestCheckRelRejects(t *testing.T) {
	tests := []struct{ name, rel string }{
		{"empty", ""},
		{"dot", "."},
		{"dotdot", ".."},
		{"parent", "../secret.song"},
		{"deep parent", "../../etc/passwd.song"},
		{"embedded parent", "Artist/../../etc/passwd.song"},
		{"trailing parent", "Artist/..song/../..song"},
		{"absolute", "/etc/passwd.song"},
		{"absolute song", "/songs/x.song"},
		{"backslash", `..\..\secret.song`},
		{"backslash sep", `Artist\Song.song`},
		{"drive letter", "C:/windows/x.song"},
		{"nul byte", "Artist/So\x00ng.song"},
		{"newline", "Artist/So\nng.song"},
		{"tab", "Artist/So\tng.song"},
		{"del", "Artist/So\x7fng.song"},
		{"double slash", "Artist//Song.song"},
		{"trailing slash", "Artist/Song.song/"},
		{"leading slash", "/Artist/Song.song"},
		{"empty segment", "/x.song"},
		{"too deep", "a/b/c/d/e/f/g/h/i.song"},
		{"no extension", "Artist/Song"},
		{"wrong extension", "Artist/Song.txt"},
		{"extension only", ".song"},
		{"extension only nested", "Artist/.song"},
		{"padded segment", "Artist / Song.song"},
		{"trailing space segment", "Artist /Song.song"},
		{"invalid utf8", "Artist/\xff\xfe.song"},
		{"dot segment mid", "Artist/./Song.song"},
		{"segment too long", strings.Repeat("a", 260) + ".song"},
		{"path too long", strings.Repeat("ab/", 400) + "x.song"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRel(tt.rel)
			if err == nil {
				t.Fatalf("CheckRel(%q) = nil, want an error", tt.rel)
			}
			if !errors.Is(err, ErrNotLocal) {
				t.Errorf("CheckRel(%q) error = %v, want it to wrap ErrNotLocal", tt.rel, err)
			}
		})
	}
}

func TestSanitizeSegment(t *testing.T) {
	tests := []struct{ in, want string }{
		// Ordinary titles survive untouched, accents and punctuation included.
		{"Amazing Grace", "Amazing Grace"},
		{"Ain't No Sunshine", "Ain't No Sunshine"},
		{"Y nos dieron las diez", "Y nos dieron las diez"},
		{"Sigur Rós", "Sigur Rós"},
		{"Björk", "Björk"},

		// Path separators and Windows-illegal characters become hyphens.
		{"AC/DC", "AC-DC"},
		{`Artist\Name`, "Artist-Name"},
		{"Who? What!", "Who- What!"},
		{`He said "hi"`, "He said -hi"}, // the trailing separator is trimmed
		{"a:b*c?d<e>f|g", "a-b-c-d-e-f-g"},

		// Traversal attempts cannot survive as path syntax.
		{"..", "untitled"},
		{"../../etc/passwd", "etc-passwd"},
		{"/", "untitled"},
		{"...", "untitled"},

		// No dotfiles, no trailing dots or spaces (invalid on Windows).
		{".hidden", "hidden"},
		{"trailing.", "trailing"},
		{"  padded  ", "padded"},
		{"trailing ", "trailing"},

		// Control characters become spaces so words do not run together.
		{"two\nlines", "two lines"},
		{"tab\there", "tab here"},
		{"nul\x00byte", "nul byte"},

		// Runs collapse.
		{"a    b", "a b"},
		{"a----b", "a-b"},
		{"a - - b", "a - - b"},

		// Windows device names get out of the way.
		{"NUL", "_NUL"},
		{"con", "_con"},
		{"aux.txt", "_aux.txt"},
		{"COM1", "_COM1"},
		{"lpt9", "_lpt9"},
		{"console", "console"}, // not reserved, must not be mangled

		// Never empty.
		{"", "untitled"},
		{"   ", "untitled"},
		{"---", "untitled"},
	}
	for _, tt := range tests {
		if got := SanitizeSegment(tt.in); got != tt.want {
			t.Errorf("SanitizeSegment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeSegmentTruncatesOnRuneBoundary(t *testing.T) {
	// Three-byte runes, so a naive byte cut would split one in half.
	in := strings.Repeat("é", 200)
	got := SanitizeSegment(in)
	if len(got) > maxSegmentBytes {
		t.Errorf("length = %d bytes, want at most %d", len(got), maxSegmentBytes)
	}
	if !utf8.ValidString(got) {
		t.Errorf("SanitizeSegment produced invalid UTF-8: %q", got)
	}
}

func TestRelPathFor(t *testing.T) {
	tests := []struct{ artist, title, want string }{
		{"Traditional", "Amazing Grace", "Traditional/Amazing Grace.song"},
		{"", "Amazing Grace", "Amazing Grace.song"},
		{"   ", "Amazing Grace", "Amazing Grace.song"},
		{"AC/DC", "T.N.T.", "AC-DC/T.N.T.song"}, // a trailing dot is invalid on Windows
		{"../../etc", "passwd", "etc/passwd.song"},
		{"..", "..", "untitled/untitled.song"},
		{"", "", "untitled.song"},
		{"Bill Withers", "Ain't No Sunshine", "Bill Withers/Ain't No Sunshine.song"},
	}
	for _, tt := range tests {
		got := RelPathFor(tt.artist, tt.title)
		if got != tt.want {
			t.Errorf("RelPathFor(%q, %q) = %q, want %q", tt.artist, tt.title, got, tt.want)
		}
		// Whatever it produces must be a path the rest of the package accepts.
		if err := CheckRel(got); err != nil {
			t.Errorf("RelPathFor(%q, %q) = %q, which CheckRel rejects: %v", tt.artist, tt.title, got, err)
		}
	}
}

func TestWithSuffix(t *testing.T) {
	tests := []struct {
		rel  string
		n    int
		want string
	}{
		{"Song.song", 2, "Song (2).song"},
		{"Artist/Song.song", 3, "Artist/Song (3).song"},
		{"Artist/T.N.T..song", 2, "Artist/T.N.T. (2).song"},
	}
	for _, tt := range tests {
		if got := withSuffix(tt.rel, tt.n); got != tt.want {
			t.Errorf("withSuffix(%q, %d) = %q, want %q", tt.rel, tt.n, got, tt.want)
		}
	}
}

// Whatever a user types, the generated segment must be a single safe path
// component. This is the property the write path depends on.
func FuzzSanitizeSegment(f *testing.F) {
	for _, s := range []string{
		"", "..", "../../etc/passwd", "NUL", ".hidden", "AC/DC", "Sigur Rós",
		"a\x00b", strings.Repeat("é", 200), "\xff\xfe", "con.song", "   ",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := SanitizeSegment(in)
		if got == "" {
			t.Fatal("returned an empty segment")
		}
		if len(got) > maxSegmentBytes {
			t.Fatalf("returned %d bytes, limit is %d", len(got), maxSegmentBytes)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("returned invalid UTF-8: %q", got)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Fatalf("returned a path separator: %q", got)
		}
		if strings.ContainsRune(got, 0) {
			t.Fatalf("returned a NUL byte: %q", got)
		}
		if got == "." || got == ".." {
			t.Fatalf("returned %q", got)
		}
		if strings.HasPrefix(got, ".") {
			t.Fatalf("returned a dotfile: %q", got)
		}
		if got != strings.Trim(got, " ") {
			t.Fatalf("returned padded text: %q", got)
		}
		if strings.HasSuffix(got, ".") {
			t.Fatalf("returned a trailing dot: %q", got)
		}
		if base := strings.ToLower(strings.TrimSuffix(got, path.Ext(got))); windowsReserved[base] {
			t.Fatalf("returned a Windows device name: %q", got)
		}
		// And it must compose into a path the package accepts.
		if err := CheckRel(got + Ext); err != nil {
			t.Fatalf("CheckRel(%q) = %v", got+Ext, err)
		}
	})
}
