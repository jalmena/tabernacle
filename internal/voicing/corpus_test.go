package voicing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/song"
)

// The regression net that speaks in the terms that matter: every chord in a real
// song must have a real shape. A hole in the dataset shows up here as a song the
// library cannot fully illustrate, rather than as a missing table entry nobody
// notices.
func TestEveryChordInTheExamplesHasAShape(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "songs")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("no example songs in this checkout")
	}

	found := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".song") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s, err := song.Parse(data)
		if err != nil {
			t.Errorf("%s does not parse: %v", d.Name(), err)
			return nil
		}
		found++
		for _, c := range s.Chords() {
			res := For(c, Options{})
			if !res.Found() {
				t.Errorf("%s: %s has no shape at all", d.Name(), c)
				continue
			}
			if res.Warning != "" {
				t.Errorf("%s: %s produced a warning: %s", d.Name(), c, res.Warning)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("no example songs were checked")
	}
}
