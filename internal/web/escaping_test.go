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
			// This file names the types in order to forbid them.
			if e.Name() == "escaping_test.go" {
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
