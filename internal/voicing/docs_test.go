package voicing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goneFromTheModel names vocabulary the lookup deliberately does not have.
//
// A document is read as a promise. Someone told that shapes degrade gracefully
// into nearby chords will not question a diagram that looks wrong, so a document
// describing machinery this package refuses to have is worse than no document.
//
// This catches stale names, not stale prose — it is a floor, not a ceiling.
var goneFromTheModel = []string{
	"ApproxKind",
	"Approximation",
	"BassOmitted",
	"QualitySimplified",
	"Exact and approximate",
	"approximate shape",
}

func TestTheDiagramDocumentMatchesTheModel(t *testing.T) {
	docs := filepath.Join("..", "..", "docs")
	entries, err := os.ReadDir(docs)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(docs, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, term := range goneFromTheModel {
			if strings.Contains(string(data), term) {
				t.Errorf("docs/%s describes %q, which the lookup does not do", e.Name(), term)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no documents were checked")
	}
}
