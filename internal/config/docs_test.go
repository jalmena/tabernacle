package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var envVar = regexp.MustCompile(`TABERNACLE_[A-Z_]+`)

// Every variable the application reads has to appear in the README's table.
//
// A setting nobody can find out about is a setting nobody uses, and these are
// exactly the things that get added and then forgotten. Both directions are
// checked: an undocumented variable and a documented one that no longer exists
// are both wrong.
func TestEveryVariableIsDocumented(t *testing.T) {
	code := read(t, filepath.Join("config.go"))
	readme := read(t, filepath.Join("..", "..", "README.md"))

	inCode := names(code)
	inReadme := names(readme)

	if len(inCode) < 10 {
		t.Fatalf("only %d variables were found in the source; this test proves little", len(inCode))
	}
	for name := range inCode {
		if !inReadme[name] {
			t.Errorf("%s is read by config.Load but is not in the README's table", name)
		}
	}
	for name := range inReadme {
		if !inCode[name] {
			t.Errorf("the README documents %s, which nothing reads", name)
		}
	}
}

func names(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range envVar.FindAllString(text, -1) {
		out[m] = true
	}
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToUpper(string(b))
}
