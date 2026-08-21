package account

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCost keeps the tests quick. The production cost is DefaultCost and is
// asserted separately, so lowering it here cannot lower it there.
const testCost = 2

const goodPassword = "una contraseña larga"

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), "accounts.json", testCost)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func admin(t *testing.T, s *Store, name string) User {
	t.Helper()
	u, err := s.Create(name, goodPassword, RoleAdmin)
	if err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return u
}

// A fresh instance has nobody in it, which is what lets a collection that
// predates accounts keep working until somebody claims it.
func TestAFreshStoreIsUnclaimed(t *testing.T) {
	s := open(t)
	if s.Count() != 0 {
		t.Errorf("Count = %d, want 0", s.Count())
	}
	if len(s.Secret()) < 32 {
		t.Errorf("the signing key is %d bytes", len(s.Secret()))
	}
}

// The signing key persists, which is the whole reason a session survives a
// restart.
func TestTheSigningKeySurvivesReopening(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatal(err)
	}
	want := string(first.Secret())
	first.Close()

	second, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got := string(second.Secret()); got != want {
		t.Error("the signing key changed on reopening, so every session would be lost")
	}
}

func TestCreateAndAuthenticate(t *testing.T) {
	s := open(t)
	created := admin(t, s, "jose")

	got, err := s.Authenticate("jose", goodPassword)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != created.ID || got.Name != "jose" || !got.IsAdmin() {
		t.Errorf("Authenticate returned %+v", got)
	}
	if got.Epoch != 1 {
		t.Errorf("a new account starts at epoch %d, want 1", got.Epoch)
	}
	// The name is compared without case, because nobody remembers how they
	// capitalised it.
	if _, err := s.Authenticate("JOSE", goodPassword); err != nil {
		t.Errorf("a differently-cased name failed: %v", err)
	}
}

// A wrong password and a name that does not exist must be indistinguishable,
// because telling them apart tells an attacker which names are worth attacking.
func TestAWrongPasswordAndAnUnknownNameAreTheSame(t *testing.T) {
	s := open(t)
	admin(t, s, "jose")

	_, wrong := s.Authenticate("jose", "otra contraseña larga")
	_, unknown := s.Authenticate("nadie", goodPassword)

	if !errors.Is(wrong, ErrBadCredentials) {
		t.Errorf("a wrong password gave %v", wrong)
	}
	if !errors.Is(unknown, ErrBadCredentials) {
		t.Errorf("an unknown name gave %v", unknown)
	}
	if wrong.Error() != unknown.Error() {
		t.Errorf("the two failures read differently:\n %v\n %v", wrong, unknown)
	}
}

// The two paths do the same work, so they take about the same time. The
// structural guarantee is that both derive a key at the same cost; this only
// catches that being taken out.
func TestAnUnknownNameCostsWhatAWrongPasswordCosts(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	s, err := Open(t.TempDir(), "accounts.json", 60_000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Create("jose", goodPassword, RoleAdmin); err != nil {
		t.Fatal(err)
	}

	measure := func(name, password string) time.Duration {
		best := time.Hour
		for range 3 {
			start := time.Now()
			s.Authenticate(name, password)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}
	wrong := measure("jose", "otra contraseña larga")
	unknown := measure("nadie", goodPassword)

	ratio := float64(wrong) / float64(unknown)
	if ratio < 0.25 || ratio > 4 {
		t.Errorf("a wrong password took %v and an unknown name %v (ratio %.2f)",
			wrong, unknown, ratio)
	}
}

func TestNamesAreRefusedWhenUnusable(t *testing.T) {
	s := open(t)
	for _, name := range []string{
		"", "a", strings.Repeat("a", 33), "con espacio", "acentuación",
		"<script>", "jo/se", "jose@example.com", "  ",
	} {
		if _, err := s.Create(name, goodPassword, RoleUser); !errors.Is(err, ErrNameInvalid) {
			t.Errorf("Create(%q) gave %v, want ErrNameInvalid", name, err)
		}
	}
	for _, name := range []string{"ab", "jose", "jose.david", "jose_d", "jose-d", "u2"} {
		if _, err := s.Create(name, goodPassword, RoleUser); err != nil {
			t.Errorf("Create(%q) gave %v", name, err)
		}
	}
}

func TestNamesAreUniqueWhateverTheCase(t *testing.T) {
	s := open(t)
	admin(t, s, "jose")
	if _, err := s.Create("JOSE", goodPassword, RoleUser); !errors.Is(err, ErrNameTaken) {
		t.Errorf("a differently-cased duplicate gave %v", err)
	}
}

func TestAShortPasswordIsRefused(t *testing.T) {
	s := open(t)
	if _, err := s.Create("jose", "corta", RoleAdmin); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("Create gave %v, want ErrPasswordTooShort", err)
	}
	if s.Count() != 0 {
		t.Error("a refused account was created anyway")
	}
}

// Changing a password logs out every other device, which is the only thing that
// makes changing it worth doing.
func TestChangingAPasswordBumpsTheEpoch(t *testing.T) {
	s := open(t)
	u := admin(t, s, "jose")

	if err := s.ChangePassword(u.ID, "la que no es", "otra contraseña larga"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("a change without the current password gave %v", err)
	}
	if err := s.ChangePassword(u.ID, goodPassword, "otra contraseña larga"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	after, _ := s.ByID(u.ID)
	if after.Epoch != u.Epoch+1 {
		t.Errorf("epoch went from %d to %d", u.Epoch, after.Epoch)
	}
	if _, err := s.Authenticate("jose", goodPassword); !errors.Is(err, ErrBadCredentials) {
		t.Error("the old password still works")
	}
	if _, err := s.Authenticate("jose", "otra contraseña larga"); err != nil {
		t.Errorf("the new password does not work: %v", err)
	}
}

// Renaming must not log anybody out: a token carries the id, not the name.
func TestRenamingKeepsTheEpoch(t *testing.T) {
	s := open(t)
	u := admin(t, s, "jose")

	if err := s.Rename(u.ID, "josedavid"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	after, ok := s.ByID(u.ID)
	if !ok {
		t.Fatal("the account vanished")
	}
	if after.Name != "josedavid" {
		t.Errorf("name = %q", after.Name)
	}
	if after.Epoch != u.Epoch {
		t.Errorf("renaming changed the epoch from %d to %d", u.Epoch, after.Epoch)
	}
	if _, err := s.Authenticate("josedavid", goodPassword); err != nil {
		t.Errorf("the new name does not log in: %v", err)
	}
}

// The invariants that make locking yourself out impossible. They live in this
// package rather than in a handler so a second caller cannot forget them.
func TestThereIsAlwaysAnAdministrator(t *testing.T) {
	s := open(t)
	first := admin(t, s, "jose")
	other, err := s.Create("ana", goodPassword, RoleUser)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(other.ID, first.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("deleting the last administrator gave %v", err)
	}
	if err := s.SetRole(other.ID, first.ID, RoleUser); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting the last administrator gave %v", err)
	}

	// With a second administrator, both become possible.
	if err := s.SetRole(first.ID, other.ID, RoleAdmin); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if err := s.SetRole(other.ID, first.ID, RoleUser); err != nil {
		t.Errorf("demoting with a spare administrator gave %v", err)
	}
}

func TestAnAdministratorCannotUnmakeThemselves(t *testing.T) {
	s := open(t)
	first := admin(t, s, "jose")
	admin(t, s, "ana") // so the last-administrator rule is not what refuses

	if err := s.Delete(first.ID, first.ID); !errors.Is(err, ErrSelfTarget) {
		t.Errorf("self-delete gave %v", err)
	}
	if err := s.SetRole(first.ID, first.ID, RoleUser); !errors.Is(err, ErrSelfTarget) {
		t.Errorf("self-demote gave %v", err)
	}
}

func TestThemeIsRememberedAndValidated(t *testing.T) {
	s := open(t)
	u := admin(t, s, "jose")

	for _, theme := range []string{ThemeDark, ThemeLight, ThemeSystem} {
		if err := s.SetTheme(u.ID, theme); err != nil {
			t.Errorf("SetTheme(%q): %v", theme, err)
		}
		if got, _ := s.ByID(u.ID); got.Theme != theme {
			t.Errorf("theme = %q, want %q", got.Theme, theme)
		}
	}
	if err := s.SetTheme(u.ID, "sepia"); !errors.Is(err, ErrThemeInvalid) {
		t.Errorf("an unknown theme gave %v", err)
	}
}

// Bootstrap is the documented way back in, so both halves are pinned: it creates
// when the account is absent, and resets when it is present.
func TestBootstrapCreatesThenResets(t *testing.T) {
	s := open(t)

	created, err := s.Bootstrap("jose", goodPassword)
	if err != nil || !created {
		t.Fatalf("Bootstrap = %v, %v; want true, nil", created, err)
	}
	u, _ := s.ByID(s.List()[0].ID)
	if !u.IsAdmin() {
		t.Error("the bootstrap account is not an administrator")
	}

	// Demote and forget the password, then bootstrap again.
	other := admin(t, s, "ana")
	if err := s.SetRole(other.ID, u.ID, RoleUser); err != nil {
		t.Fatal(err)
	}
	created, err = s.Bootstrap("jose", "una contraseña nueva")
	if err != nil || created {
		t.Fatalf("the second Bootstrap = %v, %v; want false, nil", created, err)
	}
	back, _ := s.ByID(u.ID)
	if !back.IsAdmin() {
		t.Error("bootstrapping did not restore the administrator role")
	}
	if back.Epoch <= u.Epoch {
		t.Error("a reset did not invalidate the old sessions")
	}
	if _, err := s.Authenticate("jose", "una contraseña nueva"); err != nil {
		t.Errorf("the reset password does not work: %v", err)
	}
}

// A password stored to weaker parameters is upgraded on the next successful
// login, and the upgrade does not log anybody out.
func TestALoginUpgradesAWeakHash(t *testing.T) {
	dir := t.TempDir()
	weak, err := Open(dir, "accounts.json", 2)
	if err != nil {
		t.Fatal(err)
	}
	u, err := weak.Create("jose", goodPassword, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	weak.Close()

	strong, err := Open(dir, "accounts.json", 500)
	if err != nil {
		t.Fatal(err)
	}
	defer strong.Close()
	if _, err := strong.Authenticate("jose", goodPassword); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "$500$") {
		t.Error("the hash was not upgraded")
	}
	after, _ := strong.ByID(u.ID)
	if after.Epoch != u.Epoch {
		t.Errorf("the upgrade changed the epoch from %d to %d", u.Epoch, after.Epoch)
	}
}

// The production cost is asserted where it cannot be lowered by a test's
// convenience.
func TestTheDefaultCostIsSerious(t *testing.T) {
	if DefaultCost < 210_000 {
		t.Errorf("DefaultCost = %d, below the current recommendation", DefaultCost)
	}
}

func TestConcurrentChangesLeaveAReadableFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatal(err)
	}
	admins := make([]User, 0, 4)
	for _, n := range []string{"a1", "a2", "a3", "a4"} {
		u, err := s.Create(n, goodPassword, RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		admins = append(admins, u)
	}

	var wg sync.WaitGroup
	for i, u := range admins {
		wg.Add(3)
		go func() { defer wg.Done(); s.SetTheme(u.ID, ThemeDark) }()
		go func() { defer wg.Done(); s.Authenticate(u.Name, goodPassword) }()
		go func() { defer wg.Done(); s.Rename(u.ID, u.Name+"x") }()
		_ = i
	}
	wg.Wait()
	s.Close()

	// Whatever the interleaving, the file has to still be a file.
	reopened, err := Open(dir, "accounts.json", testCost)
	if err != nil {
		t.Fatalf("the file is unreadable after concurrent changes: %v", err)
	}
	defer reopened.Close()
	if reopened.Count() != 4 {
		t.Errorf("Count = %d, want 4", reopened.Count())
	}
}
