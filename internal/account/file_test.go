package account

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The file holds password hashes and the key that signs every session, so it is
// not a world-readable file.
func TestTheFileIsPrivateAndLeavesNoTemporaries(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Create("jose", goodPassword, RoleAdmin); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("mode = %o, want 600", perm)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d entries, want just the accounts file", len(entries))
	}
}

// A file that cannot be understood must not be read as an empty one: that would
// silently return the instance to unclaimed mode and let anybody write.
func TestACorruptFileIsRefusedRatherThanEmptied(t *testing.T) {
	valid := `{"version":1,"secret":"` + mustSecret(t) + `","users":[` + oneUser("jose", "admin") + `]}`

	tests := map[string]string{
		"not json":          `{`,
		"empty":             ``,
		"no version":        `{"secret":"x","users":[]}`,
		"future version":    `{"version":2,"secret":"` + mustSecret(t) + `","users":[]}`,
		"unknown field":     strings.Replace(valid, `"users"`, `"admins"`, 1),
		"short secret":      `{"version":1,"secret":"c2hvcnQ","users":[]}`,
		"unreadable secret": `{"version":1,"secret":"!!!","users":[]}`,
		"unknown role":      strings.Replace(valid, `"admin"`, `"root"`, 1),
		"no id":             strings.Replace(valid, `"id":"aaaabbbbccccdddd"`, `"id":""`, 1),
		"no hash":           strings.Replace(valid, fixedHash, "", 1),
		"epoch zero":        strings.Replace(valid, `"epoch":1`, `"epoch":0`, 1),
		"unusable name":     strings.Replace(valid, `"name":"jose"`, `"name":"jo se"`, 1),
		"unknown theme":     strings.Replace(valid, `"epoch":1`, `"theme":"sepia","epoch":1`, 1),
		"duplicate name": `{"version":1,"secret":"` + mustSecret(t) + `","users":[` +
			oneUser("jose", "admin") + `,` + oneUser("JOSE", "user") + `]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, body)
			s, err := Open(dir, "accounts.json", testCost)
			if err == nil {
				t.Errorf("Open accepted it, with %d accounts", s.Count())
				s.Close()
				return
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Errorf("Open gave %v, want ErrCorrupt", err)
			}
		})
	}

	// And the valid one it is derived from does load, or the test above proves
	// only that the fixture is broken.
	t.Run("the valid fixture", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, valid)
		s, err := Open(dir, "accounts.json", testCost)
		if err != nil {
			t.Fatalf("Open refused the valid fixture: %v", err)
		}
		defer s.Close()
		if s.Count() != 1 {
			t.Errorf("Count = %d, want 1", s.Count())
		}
	})
}

// Editing the file by hand is a documented way back in, so it has to work.
func TestAHandEditedFileLoads(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.Create("jose", goodPassword, RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Promote by hand, the way somebody with a shell would.
	path := filepath.Join(dir, "accounts.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), `"role": "user"`, `"role": "admin"`, 1)
	if edited == string(raw) {
		t.Fatalf("the file does not look the way this test expects:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	again, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer again.Close()
	got, ok := again.ByID(u.ID)
	if !ok || !got.IsAdmin() {
		t.Errorf("the hand-edited role did not take: %+v", got)
	}
	if _, err := again.Authenticate("jose", goodPassword); err != nil {
		t.Errorf("the password stopped working: %v", err)
	}
}

func TestOpenRefusesAnythingButAPlainFilename(t *testing.T) {
	for _, name := range []string{"", ".", "..", "sub/accounts.json", "../accounts.json", `a\b`} {
		if s, err := Open(t.TempDir(), name, testCost); err == nil {
			s.Close()
			t.Errorf("Open accepted the name %q", name)
		}
	}
}

func mustSecret(t *testing.T) string {
	t.Helper()
	s, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func oneUser(name, role string) string {
	return `{"id":"aaaabbbbccccdddd","name":"` + name + `","role":"` + role +
		`","hash":"` + fixedHash + `","epoch":1,"created_at":"2026-01-01T00:00:00Z"}`
}

// fixedHash is a real hash of goodPassword at testCost, written out so the
// fixtures above are plain strings a reader can see.
var fixedHash = func() string {
	h, err := hashPassword(goodPassword, testCost)
	if err != nil {
		panic(err)
	}
	return h
}()
