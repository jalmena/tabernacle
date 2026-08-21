package song

import (
	"reflect"
	"testing"
)

// FuzzParse checks the two properties that matter for imported files, which
// are untrusted input: Parse never panics, and anything it accepts survives a
// write and a read back unchanged.
func FuzzParse(f *testing.F) {
	f.Add("---\ntitle: T\n---\n\n## Verse 1\n[C]hello [x2]\n")
	f.Add("---\ntitle: \"Yes\"\ngenre: rock\nnotes: |-\n  one\n  two\n---\n")
	f.Add("---\ntitle: T\n---\n\n## Solo\n```tab\ne|-0-|\n```\n")
	f.Add("---\ntitle: T\nkey: Bb\ncapo: 3\n---\n[Bb]x\\[y\\\\z\n")
	f.Add("---\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, src string) {
		first, err := Parse([]byte(src))
		if err != nil {
			return
		}
		out, err := Marshal(first)
		if err != nil {
			t.Fatalf("Marshal rejected a song Parse accepted: %v\ninput: %q", err, src)
		}
		second, err := Parse(out)
		if err != nil {
			t.Fatalf("Parse rejected its own output: %v\ninput: %q\noutput: %q", err, src, out)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("round trip changed the song\ninput: %q\noutput: %q\nfirst: %+v\nsecond: %+v",
				src, out, first, second)
		}
		again, err := Marshal(second)
		if err != nil {
			t.Fatalf("Marshal failed on a reparsed song: %v", err)
		}
		if string(again) != string(out) {
			t.Fatalf("Marshal is not idempotent\ninput: %q\nfirst: %q\nsecond: %q", src, out, again)
		}
	})
}
