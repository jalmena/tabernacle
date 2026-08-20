package song

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleSongsAreCanonical parses the demo content shipped in the
// repository and checks it is written in canonical form.
//
// It keeps the examples honest as the format grows, and it makes them usable as
// documentation: what is in examples/ is exactly what the writer produces.
func TestExampleSongsAreCanonical(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "songs")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no example songs to check: %v", err)
	}

	found := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".song") {
			return err
		}
		found++
		t.Run(filepath.Base(path), func(t *testing.T) {
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			s, err := Parse(want)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := Marshal(s)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("not canonical\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if found == 0 {
		t.Errorf("no .song files found under %s", root)
	}
}
