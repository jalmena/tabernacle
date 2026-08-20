package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule is that html/template does every bit of the escaping, which only
// holds while nothing in these packages hands it a pre-trusted string. That is
// an architectural constraint, and a constraint nobody enforces is a comment,
// so this turns it into a build failure.
func TestNoPreTrustedMarkupTypes(t *testing.T) {
	forbidden := []string{
		"template.HTML",
		"template.JS",
		"template.URL",
		"template.CSS",
		"template.HTMLAttr",
		"template.JSStr",
		"template.Srcset",
	}
	for _, dir := range []string{".", "../render"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			switch e.Name() {
			case "escaping_test.go":
				continue // this file names the types in order to forbid them
			case "diagram.go":
				// The one documented exception: chord diagrams are generated
				// from integers by a package that takes no markup and escapes
				// the single string it does take. Checked separately below.
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, bad := range forbidden {
				if strings.Contains(string(data), bad) {
					t.Errorf("%s uses %s; every value handed to a template must be a plain "+
						"string so html/template escapes it in the right context", path, bad)
				}
			}
		}
	}
}

// The diagram exception must stay one function in one file. If pre-escaped
// markup starts spreading, the rule above has quietly stopped meaning anything.
func TestOnlyDiagramProducesMarkup(t *testing.T) {
	data, err := os.ReadFile("diagram.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if n := strings.Count(body, "template.HTML"); n != 2 {
		// Once in the signature, once in the conversion.
		t.Errorf("diagram.go mentions template.HTML %d times, want exactly 2", n)
	}
	if !strings.Contains(body, "func chordDiagram(") {
		t.Error("diagram.go no longer defines chordDiagram; the exception is unanchored")
	}
	// It must not touch anything that could carry user markup other than the
	// label, which the diagram package escapes itself.
	for _, forbidden := range []string{"template.JS", "template.URL", "template.CSS"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("diagram.go uses %s", forbidden)
		}
	}
}

// No inline script or style, which is what lets the content security policy do
// without nonces, hashes or unsafe-inline.
func TestTemplatesHaveNoInlineScriptOrStyle(t *testing.T) {
	entries, err := os.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("templates", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		// A script tag is only allowed when it is a src reference.
		for _, chunk := range strings.Split(body, "<script")[1:] {
			head := chunk
			if i := strings.Index(head, ">"); i >= 0 {
				head = head[:i]
			}
			if !strings.Contains(head, "src=") {
				t.Errorf("%s has an inline script", e.Name())
			}
		}
		if strings.Contains(body, "<style") {
			t.Errorf("%s has an inline style block", e.Name())
		}
		if strings.Contains(body, "onclick=") || strings.Contains(body, "onload=") {
			t.Errorf("%s has an inline event handler", e.Name())
		}
	}
}
