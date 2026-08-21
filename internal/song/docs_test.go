package song

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// retiredNames are the header fields the format no longer carries. They are
// still accepted on the way in, so nothing breaks, but they must not appear in
// anything this repository ships as an example — a document that teaches a field
// which no longer exists is worse than no document.
var retiredNames = []string{"format", "tempo", "meter", "tags"}

func TestShippedSongsCarryNoRetiredFields(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "examples"),
		"testdata",
	}
	checked := 0
	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".song") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			checked++
			if _, err := Parse(data); err != nil {
				t.Errorf("%s does not parse: %v", path, err)
				return nil
			}
			for _, field := range retiredNames {
				if headerHas(string(data), field) {
					t.Errorf("%s still carries the retired field %q", path, field)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no song files were checked")
	}
}

// headerHas reports whether the metadata block sets a field.
func headerHas(content, field string) bool {
	body, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return false
	}
	header, _, _ := strings.Cut(body, "\n---")
	for _, line := range strings.Split(header, "\n") {
		if name, _, found := strings.Cut(line, ":"); found &&
			strings.TrimSpace(name) == field {
			return true
		}
	}
	return false
}

// fencedBlock matches a code fence of three or more backticks, so an example
// containing a ```tab block is captured whole.
var fencedBlock = regexp.MustCompile("(?s)```(?:`*)[a-z]*\n(.*?)\n```(?:`*)")

// Every .song example in the documentation must be a real one: it has to parse,
// and it must not teach a field that has been retired. Documentation examples
// rot silently, and a reader copying one is entitled to have it work.
func TestDocumentationExamplesAreRealSongs(t *testing.T) {
	docs := []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "docs", "song-format.md"),
		filepath.Join("..", "..", "docs", "chord-diagrams.md"),
		filepath.Join("..", "..", "docs", "deployment.md"),
		filepath.Join("..", "..", "docs", "printing.md"),
	}
	found := 0
	for _, doc := range docs {
		data, err := os.ReadFile(doc)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		for _, m := range fencedBlock.FindAllStringSubmatch(string(data), -1) {
			block := m[1]
			// Only the blocks that are song files: they open with the metadata
			// delimiter.
			if !strings.HasPrefix(block, "---\n") {
				continue
			}
			found++
			if !strings.HasSuffix(block, "\n") {
				block += "\n"
			}
			if _, err := Parse([]byte(block)); err != nil {
				t.Errorf("%s: an example does not parse: %v\n%s", doc, err, block)
				continue
			}
			for _, field := range retiredNames {
				if headerHas(block, field) {
					t.Errorf("%s: an example still teaches the retired field %q", doc, field)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no song examples were found in the documentation")
	}
}
