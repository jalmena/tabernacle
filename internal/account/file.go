package account

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jalmena/tabernacle/internal/i18n"
)

// fileVersion is the shape of what is on disk. A file from the future is refused
// rather than read half-right.
const fileVersion = 1

// fileMode keeps the accounts file to its owner. It holds password hashes and the
// key that signs every session, so it is not a 0o644 sort of file.
const fileMode fs.FileMode = 0o600

type fileData struct {
	Version int    `json:"version"`
	Secret  string `json:"secret"`
	// Registration, when true, lets a visitor create their own account. Off
	// unless an administrator turns it on.
	//
	// It lives here rather than in the settings table because it decides who may
	// write, and the index is disposable: losing it must not quietly change the
	// answer.
	Registration bool         `json:"registration,omitempty"`
	Users        []storedUser `json:"users"`
}

// storedUser is a user as written. It is separate from User because User is what
// callers see, and a password hash is not.
type storedUser struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Role        string    `json:"role"`
	Hash        string    `json:"hash"`
	Theme       string    `json:"theme,omitempty"`
	Lang        string    `json:"lang,omitempty"`
	Epoch       int       `json:"epoch"`
	CreatedAt   time.Time `json:"created_at"`
	LastLoginAt time.Time `json:"last_login_at,omitzero"`
}

func (u storedUser) view() User {
	return User{
		ID:          u.ID,
		Name:        u.Name,
		Role:        Role(u.Role),
		Theme:       u.Theme,
		Lang:        u.Lang,
		Epoch:       u.Epoch,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

// readFile loads and validates the accounts file.
//
// Everything it can refuse, it refuses loudly. A file it cannot understand must
// not be read as an empty one: that would silently return the instance to
// unclaimed mode and let anybody write to the collection.
func readFile(root *os.Root, name string) (fileData, error) {
	f, err := root.Open(name)
	if err != nil {
		return fileData{}, err
	}
	defer f.Close()

	var d fileData
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return fileData{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if d.Version != fileVersion {
		return fileData{}, fmt.Errorf("%w: version %d is not supported", ErrCorrupt, d.Version)
	}
	if _, err := decodeSecret(d.Secret); err != nil {
		return fileData{}, fmt.Errorf("%w: the signing key is unreadable", ErrCorrupt)
	}

	seen := map[string]bool{}
	for i, u := range d.Users {
		switch {
		case u.ID == "":
			return fileData{}, fmt.Errorf("%w: user %d has no id", ErrCorrupt, i)
		case !validName(u.Name):
			return fileData{}, fmt.Errorf("%w: %q is not a usable name", ErrCorrupt, u.Name)
		case Role(u.Role) != RoleAdmin && Role(u.Role) != RoleUser:
			return fileData{}, fmt.Errorf("%w: %q has the unknown role %q", ErrCorrupt, u.Name, u.Role)
		case u.Hash == "":
			return fileData{}, fmt.Errorf("%w: %q has no password", ErrCorrupt, u.Name)
		case u.Epoch < 1:
			return fileData{}, fmt.Errorf("%w: %q has epoch %d", ErrCorrupt, u.Name, u.Epoch)
		case !validTheme(u.Theme):
			return fileData{}, fmt.Errorf("%w: %q has the unknown theme %q", ErrCorrupt, u.Name, u.Theme)
		case u.Lang != "" && !i18n.Lang(u.Lang).Valid():
			return fileData{}, fmt.Errorf("%w: %q has the unknown language %q", ErrCorrupt, u.Name, u.Lang)
		case seen[fold(u.Name)], seen[u.ID]:
			return fileData{}, fmt.Errorf("%w: %q appears twice", ErrCorrupt, u.Name)
		}
		seen[fold(u.Name)], seen[u.ID] = true, true
	}
	return d, nil
}

// writeFile replaces the accounts file atomically.
//
// A temporary file in the same directory, then a rename, so a crash or a full
// disk leaves the previous accounts intact rather than a half-written file that
// nobody can log in against.
func writeFile(root *os.Root, name string, d fileData) error {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("account: encode: %w", err)
	}
	raw = append(raw, '\n')

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("account: name a temporary file: %w", err)
	}
	tmp := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"

	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return fmt.Errorf("account: create %s: %w", tmp, err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		_ = root.Remove(tmp)
		return fmt.Errorf("account: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = root.Remove(tmp)
		return fmt.Errorf("account: sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("account: close %s: %w", tmp, err)
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("account: replace %s: %w", name, err)
	}
	syncDir(root)
	return nil
}

// syncDir asks the filesystem to record the rename. Best effort: a failure here
// costs durability on a power cut, not correctness now.
func syncDir(root *os.Root) {
	if f, err := root.Open("."); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
}

// checkFilename refuses anything that is not a plain name in the data directory.
// The confined root would refuse an escape anyway; this says why.
func checkFilename(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return fmt.Errorf("account: %q is not a filename", name)
	}
	return nil
}

var errNoSecret = errors.New("account: empty signing key")
