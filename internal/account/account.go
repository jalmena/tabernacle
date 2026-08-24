// Package account holds the people who may change the collection.
//
// Accounts live in a JSON file beside the songs, not in the SQLite index. The
// index is documented and tested as disposable — delete it and the next start
// rebuilds it from the .song files — and an account is not recoverable from a
// song. Putting accounts there would quietly make that promise conditional. A
// file is also backed up by copying the data directory, and can be edited by hand
// when somebody has locked themselves out.
//
// This package knows nothing about HTTP. It holds no cookies, reads no headers
// and renders no messages; internal/web owns all of that. What lives here are the
// rules that must hold however they are reached: who exists, what they may be
// called, how a password is stored, and that there is always an administrator.
package account

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jalmena/tabernacle/internal/i18n"
)

// Role is what an account is allowed to do.
//
// Two, and no more. A songbook for a household does not need a permission
// matrix, and every extra role doubles what has to be reasoned about.
type Role string

const (
	// RoleAdmin may manage accounts and settings, as well as songs.
	RoleAdmin Role = "admin"
	// RoleUser may add, edit and delete songs.
	RoleUser Role = "user"
)

// Themes a user may choose. The empty string follows the reader's system.
const (
	ThemeSystem = ""
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

var (
	// ErrBadCredentials is returned for a wrong password and for a name that does
	// not exist. One value, deliberately: telling those apart tells an attacker
	// which names are worth attacking.
	ErrBadCredentials = errors.New("account: the username or password is wrong")

	ErrNotFound         = errors.New("account: no such account")
	ErrNameTaken        = errors.New("account: that name is already in use")
	ErrNameInvalid      = errors.New("account: that name cannot be used")
	ErrPasswordTooShort = fmt.Errorf("account: a password needs at least %d characters", MinPasswordRunes)
	ErrThemeInvalid     = errors.New("account: no such theme")
	ErrLangInvalid      = errors.New("account: no such language")
	ErrNotationInvalid  = errors.New("account: no such notation")
	ErrCorrupt          = errors.New("account: the accounts file cannot be read")

	// ErrLastAdmin refuses the change that would leave nobody in charge.
	ErrLastAdmin = errors.New("account: this is the last administrator")
	// ErrSelfTarget refuses an administrator the two changes they cannot undo on
	// themselves.
	ErrSelfTarget = errors.New("account: an administrator cannot do that to their own account")
)

// User is an account as everything outside this package sees it. No hash.
type User struct {
	ID    string
	Name  string
	Role  Role
	Theme string
	// Lang is empty when this person has not chosen one, which means the
	// interface's own default.
	Lang string
	// Notation is empty when this person has not chosen how notes are named,
	// which means the instance's default.
	Notation    string
	Epoch       int
	CreatedAt   time.Time
	LastLoginAt time.Time
}

// IsAdmin is asked often enough to be worth a name.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// Store is the accounts file, held open.
//
// The whole file is kept in memory behind a read-write mutex: it is a handful of
// records, every request consults it, and re-reading a file per request would be
// silly. Every change writes the file first and swaps the snapshot second, which
// is the same ordering rule internal/library states for songs — a write that
// failed must not be visible.
type Store struct {
	root *os.Root
	name string
	cost int

	mu   sync.RWMutex
	data fileData

	// now is a seam for tests. Nothing else sets it.
	now func() time.Time
}

// Open opens the accounts file, creating an empty one if it is not there.
//
// An empty file is not a failure: it is an unclaimed instance, which is how a
// collection that predates accounts keeps working until somebody claims it. The
// signing key is generated once, at that moment, and persists — which is what
// lets a session survive a restart.
//
// cost is the PBKDF2 iteration count for new passwords; production passes
// DefaultCost.
func Open(dir, name string, cost int) (*Store, error) {
	if err := checkFilename(name); err != nil {
		return nil, err
	}
	if cost < 1 {
		return nil, errors.New("account: the hashing cost must be positive")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("account: create %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("account: open %s: %w", dir, err)
	}

	s := &Store{root: root, name: name, cost: cost, now: time.Now}

	switch d, err := readFile(root, name); {
	case err == nil:
		s.data = d
	case errors.Is(err, os.ErrNotExist):
		secret, err := newSecret()
		if err != nil {
			root.Close()
			return nil, err
		}
		s.data = fileData{Version: fileVersion, Secret: secret}
		if err := writeFile(root, name, s.data); err != nil {
			root.Close()
			return nil, err
		}
	default:
		root.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the directory handle.
func (s *Store) Close() error { return s.root.Close() }

// Secret is the key that signs session tokens.
func (s *Store) Secret() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := decodeSecret(s.data.Secret)
	return b
}

// RegistrationOpen reports whether a visitor may create their own account.
func (s *Store) RegistrationOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Registration
}

// SetRegistrationOpen opens or closes registration.
//
// Closed unless an administrator says otherwise: a songbook on a home network
// that anybody can sign up to is not what most people mean by one.
func (s *Store) SetRegistrationOpen(open bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.clone()
	next.Registration = open
	return s.commit(next)
}

// Count is how many accounts exist. Zero means unclaimed.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Users)
}

// List is every account, by name, for an administrator's table.
func (s *Store) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		out = append(out, u.view())
	}
	slices.SortFunc(out, func(a, b User) int { return strings.Compare(fold(a.Name), fold(b.Name)) })
	return out
}

// ByID finds an account. This is the lookup a session cookie needs.
func (s *Store) ByID(id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := s.indexOfID(id); i >= 0 {
		return s.data.Users[i].view(), true
	}
	return User{}, false
}

// Authenticate checks a name and password.
//
// A name that does not exist costs the same as a wrong password: the work is done
// against a decoy hash so that response time does not report which names are
// real. Both return ErrBadCredentials.
//
// A successful login also re-hashes a password stored to weaker parameters, and
// records the time. Neither changes the epoch: upgrading a hash must not log
// somebody out of their other devices.
func (s *Store) Authenticate(name, password string) (User, error) {
	s.mu.RLock()
	cost := s.cost
	var stored storedUser
	found := false
	if i := s.indexOfName(name); i >= 0 {
		stored, found = s.data.Users[i], true
	}
	s.mu.RUnlock()

	// The hashing happens with no lock held. It takes the better part of a second
	// by design, and every page load reads this store, so holding the lock across
	// it would let one login attempt stall the whole site.
	if !found {
		burn(cost)
		return User{}, ErrBadCredentials
	}
	if !verifyPassword(stored.Hash, password) {
		return User{}, ErrBadCredentials
	}

	upgraded := ""
	if needsRehash(stored.Hash, cost) {
		if h, err := hashPassword(password, cost); err == nil {
			upgraded = h
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(stored.ID)
	if i < 0 {
		// Deleted while we were hashing. The credentials were right; the account
		// is gone, and gone wins.
		return User{}, ErrBadCredentials
	}
	next := s.clone()
	next.Users[i].LastLoginAt = s.now()
	// Only replace the hash we actually checked: a password changed on another
	// device while this one was hashing must not be reverted.
	if upgraded != "" && next.Users[i].Hash == stored.Hash {
		next.Users[i].Hash = upgraded
	}
	if err := s.commit(next); err != nil {
		// The login itself was good and only the bookkeeping failed. Refusing
		// here would lock everybody out of a readable collection because a disk
		// filled up, which is the wrong trade.
		return s.data.Users[i].view(), nil
	}
	return s.data.Users[i].view(), nil
}

// Create adds an account. Only an administrator reaches this, and there is no
// registration: somebody who is already trusted decides who else is.
func (s *Store) Create(name, password string, role Role) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.create(name, password, role)
}

func (s *Store) create(name, password string, role Role) (User, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return User{}, ErrNameInvalid
	}
	if role != RoleAdmin && role != RoleUser {
		return User{}, fmt.Errorf("account: unknown role %q", role)
	}
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	if s.indexOfName(name) >= 0 {
		return User{}, ErrNameTaken
	}
	h, err := hashPassword(password, s.cost)
	if err != nil {
		return User{}, err
	}
	id, err := newID()
	if err != nil {
		return User{}, err
	}

	next := s.clone()
	next.Users = append(next.Users, storedUser{
		ID:        id,
		Name:      name,
		Role:      string(role),
		Hash:      h,
		Epoch:     1,
		CreatedAt: s.now(),
	})
	if err := s.commit(next); err != nil {
		return User{}, err
	}
	return s.data.Users[len(s.data.Users)-1].view(), nil
}

// Bootstrap creates the administrator named in the environment, or resets that
// administrator's password if the account already exists.
//
// Resetting is the point. It is the documented way back in after a forgotten
// password, and it demands shell access to the host, which is the right bar for
// it. It reports whether an account was created.
func (s *Store) Bootstrap(name, password string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if !validName(name) {
		return false, ErrNameInvalid
	}
	if err := checkPassword(password); err != nil {
		return false, err
	}

	i := s.indexOfName(name)
	if i < 0 {
		if _, err := s.create(name, password, RoleAdmin); err != nil {
			return false, err
		}
		return true, nil
	}

	h, err := hashPassword(password, s.cost)
	if err != nil {
		return false, err
	}
	next := s.clone()
	next.Users[i].Hash = h
	next.Users[i].Role = string(RoleAdmin)
	next.Users[i].Epoch++
	return false, s.commit(next)
}

// Rename changes the name an account logs in with.
//
// Sessions are unaffected: a token carries the immutable id, so nobody is logged
// out by renaming themselves, and nothing else in the application refers to a
// user by name.
func (s *Store) Rename(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		return ErrNameInvalid
	}
	if j := s.indexOfName(name); j >= 0 && j != i {
		return ErrNameTaken
	}
	next := s.clone()
	next.Users[i].Name = name
	return s.commit(next)
}

// ChangePassword replaces a password for its owner, who has to prove they know
// the current one.
//
// An unlocked laptop should not turn into a permanent takeover. The rule lives
// here rather than in a handler so that a second caller cannot forget it.
func (s *Store) ChangePassword(id, current, next string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	if !verifyPassword(s.data.Users[i].Hash, current) {
		return ErrBadCredentials
	}
	return s.setPassword(i, next)
}

// SetPassword replaces a password without knowing the old one, which is what an
// administrator resetting somebody else's does.
func (s *Store) SetPassword(id, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	return s.setPassword(i, password)
}

// setPassword bumps the epoch, so every other device is logged out. A password
// change that left old sessions alive would not be a password change.
func (s *Store) setPassword(i int, password string) error {
	if err := checkPassword(password); err != nil {
		return err
	}
	h, err := hashPassword(password, s.cost)
	if err != nil {
		return err
	}
	next := s.clone()
	next.Users[i].Hash = h
	next.Users[i].Epoch++
	return s.commit(next)
}

// SetRole promotes or demotes an account.
func (s *Store) SetRole(actorID, id string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if role != RoleAdmin && role != RoleUser {
		return fmt.Errorf("account: unknown role %q", role)
	}
	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	if actorID == id {
		// Nobody demotes themselves by accident, and nobody needs to promote
		// themselves.
		return ErrSelfTarget
	}
	if Role(s.data.Users[i].Role) == RoleAdmin && role != RoleAdmin && s.admins() < 2 {
		return ErrLastAdmin
	}
	next := s.clone()
	next.Users[i].Role = string(role)
	return s.commit(next)
}

// SetTheme records a reading preference.
//
// It belongs to the person rather than to the browser, so it follows them from
// the phone to the laptop, which is why it is here and not in a cookie.
func (s *Store) SetTheme(id, theme string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validTheme(theme) {
		return ErrThemeInvalid
	}
	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	next := s.clone()
	next.Users[i].Theme = theme
	return s.commit(next)
}

// SetLang records which language this person reads the interface in.
//
// Beside the theme rather than in a cookie, for the same reason: it belongs to
// the person, so it follows them from the phone to the laptop.
func (s *Store) SetLang(id, lang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if lang != "" && !i18n.Lang(lang).Valid() {
		return ErrLangInvalid
	}
	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	next := s.clone()
	next.Users[i].Lang = lang
	return s.commit(next)
}

// SetNotation records what this person calls the notes.
//
// A reading preference like the theme and the language, so it lives with them
// rather than with the instance. What an instance sets is only the answer for
// somebody who has not chosen.
func (s *Store) SetNotation(id, notation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch notation {
	case "", "english", "solfege":
	default:
		return ErrNotationInvalid
	}
	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	next := s.clone()
	next.Users[i].Notation = notation
	return s.commit(next)
}

// BumpEpoch invalidates every session an account has. This is "log out
// everywhere", and it is also what an administrator's password reset does.
func (s *Store) BumpEpoch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	next := s.clone()
	next.Users[i].Epoch++
	return s.commit(next)
}

// Delete removes an account.
//
// Nothing is left behind: songs carry no author, reading state is per song, and
// there is no ownership anywhere in the application.
func (s *Store) Delete(actorID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOfID(id)
	if i < 0 {
		return ErrNotFound
	}
	if actorID == id {
		return ErrSelfTarget
	}
	if Role(s.data.Users[i].Role) == RoleAdmin && s.admins() < 2 {
		return ErrLastAdmin
	}
	next := s.clone()
	next.Users = slices.Delete(next.Users, i, i+1)
	return s.commit(next)
}

// --- the bits everything above leans on ---

// commit writes the file and, only if that worked, adopts the new state.
func (s *Store) commit(next fileData) error {
	if err := writeFile(s.root, s.name, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

// clone copies the snapshot so a failed write cannot leave a half-applied change
// in memory.
func (s *Store) clone() fileData {
	out := s.data
	out.Users = slices.Clone(s.data.Users)
	return out
}

func (s *Store) indexOfID(id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(s.data.Users, func(u storedUser) bool { return u.ID == id })
}

func (s *Store) indexOfName(name string) int {
	f := fold(name)
	if f == "" {
		return -1
	}
	return slices.IndexFunc(s.data.Users, func(u storedUser) bool { return fold(u.Name) == f })
}

func (s *Store) admins() int {
	n := 0
	for _, u := range s.data.Users {
		if Role(u.Role) == RoleAdmin {
			n++
		}
	}
	return n
}

// fold is how two names are compared. Case only; deliberately not accent
// folding, which would make "José" and "Jose" the same login and invite
// look-alike names.
func fold(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// validName keeps a login name to characters that cannot be mistaken for one
// another or for markup.
func validName(name string) bool {
	if n := len([]rune(name)); n < 2 || n > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func validTheme(theme string) bool {
	switch theme {
	case ThemeSystem, ThemeLight, ThemeDark:
		return true
	}
	return false
}

func checkPassword(password string) error {
	if len([]rune(password)) < MinPasswordRunes {
		return ErrPasswordTooShort
	}
	return nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("account: read random id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("account: read random signing key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func decodeSecret(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) < 32 {
		return nil, errNoSecret
	}
	return b, nil
}
