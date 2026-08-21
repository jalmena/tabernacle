package tests

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fresh instance is unclaimed: it behaves exactly as it did before accounts
// existed, and says so. That is what lets an existing collection be upgraded
// without its owner finding the editor gone and no explanation.
func TestAnUnclaimedInstanceStillWorks(t *testing.T) {
	in := start(t, nil)
	in.seed()

	page := in.get("/")
	page.mustShow("no tiene ninguna cuenta")
	page.mustContain(`href="/songs/new"`)

	res := in.post("/songs", url.Values{"text": {newSong}})
	if !strings.HasPrefix(res.location(), "/songs/") {
		t.Fatal("an unclaimed instance refused a write")
	}
	if len(in.songFiles()) != 4 {
		t.Errorf("the collection holds %v", in.songFiles())
	}
}

// Claiming it closes it, for everybody who is not signed in.
func TestClaimingClosesTheInstance(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	// The claimant is signed in and may write.
	in.get("/").mustContain(">Ajustes<")
	if !strings.HasPrefix(in.post("/songs", url.Values{"text": {newSong}}).location(), "/songs/") {
		t.Error("the administrator cannot write")
	}

	// A browser that never logged in cannot.
	stranger := in.anon()
	strangerPage := stranger.get("/")
	strangerPage.mustContain(">Acceder<")
	strangerPage.mustNotContain(`href="/songs/new"`)
	strangerPage.mustNotContain("no tiene ninguna cuenta")

	if got := stranger.post("/songs", url.Values{"text": {newSong}}).Status; got != http.StatusUnauthorized {
		t.Errorf("a stranger's write = %d, want 401", got)
	}
	// And reading is untouched.
	stranger.get("/songs/traditional-amazing-grace").mustContain("Amazing Grace")
}

// A session outlives a restart, which is the payoff for signing it rather than
// storing it.
func TestASessionSurvivesARestart(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	in.restart()
	in.get("/").mustContain(">Ajustes<")
	if !strings.HasPrefix(in.post("/songs", url.Values{"text": {newSong}}).location(), "/songs/") {
		t.Error("the session did not survive the restart")
	}
}

// And it outlives losing the index, which is a documented, supported operation.
func TestASessionSurvivesLosingTheIndex(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	in.stop()
	matches, err := filepath.Glob(filepath.Join(in.dir, "*.db*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no database was found to delete")
	}
	for _, m := range matches {
		// The accounts are not a database file, so nothing here may match them.
		if strings.HasSuffix(m, "accounts.json") {
			t.Fatalf("%s was matched by the index's own glob", m)
		}
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(in.dir, "accounts.json")); err != nil {
		t.Fatalf("the accounts file did not survive: %v", err)
	}
	in.boot()

	in.get("/").mustContain(">Ajustes<")
	in.get("/").mustContain("Amazing Grace")
}

// Logging out ends it, and the next write is refused.
func TestLoggingOut(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")
	in.logout()

	in.get("/").mustContain(">Acceder<")
	if got := in.post("/songs", url.Values{"text": {newSong}}).Status; got != http.StatusUnauthorized {
		t.Errorf("a write after logging out = %d, want 401", got)
	}
	// And logging back in works.
	in.login("jose")
	in.get("/").mustContain(">Ajustes<")
}

// A password change reaches every other device. Two jars, which is two browsers.
func TestChangingAPasswordSignsOutTheOtherDevices(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	phone := in.anon()
	phone.login("jose")
	phone.get("/").mustContain(">Ajustes<")

	res := in.post("/settings/password", url.Values{
		"current":  {testPassword},
		"password": {"una contraseña nueva"},
		"repeat":   {"una contraseña nueva"},
	})
	if res.Status != http.StatusSeeOther {
		t.Fatalf("changing the password = %d\n%.400s", res.Status, res.Body)
	}

	// This browser stays in; the other does not.
	in.get("/").mustContain(">Ajustes<")
	phone.get("/").mustContain(">Acceder<")
}

// "Log out everywhere" is the same effect on purpose, without changing anything
// you have to remember.
func TestLoggingOutEverywhere(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	phone := in.anon()
	phone.login("jose")

	if got := in.post("/settings/sessions/revoke", nil).Status; got != http.StatusSeeOther {
		t.Fatalf("revoking = %d", got)
	}
	in.get("/").mustContain(">Ajustes<")
	phone.get("/").mustContain(">Acceder<")
}

// The shape of the thing: an administrator makes accounts, and a plain account
// writes songs and nothing else.
func TestAnAdministratorMakesAnAccountThatCanWrite(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	if got := in.post("/settings/users", url.Values{
		"user": {"ana"}, "password": {testPassword}, "role": {"user"},
	}).Status; got != http.StatusSeeOther {
		t.Fatalf("creating an account = %d", got)
	}

	ana := in.anon()
	ana.login("ana")
	if !strings.HasPrefix(ana.post("/songs", url.Values{"text": {newSong}}).location(), "/songs/") {
		t.Error("a plain account cannot write a song")
	}
	if got := ana.get("/settings/users").Status; got != http.StatusForbidden {
		t.Errorf("a plain account reached the accounts page with %d", got)
	}
	if got := ana.post("/settings/users", url.Values{
		"user": {"colado"}, "password": {testPassword},
	}).Status; got != http.StatusForbidden {
		t.Errorf("a plain account created an account, with %d", got)
	}
}

// Locking yourself out is not possible by mis-click.
func TestTheLastAdministratorCannotBeRemoved(t *testing.T) {
	in := start(t, nil)
	in.seed()
	in.claim("jose")

	me := adminID(t, in)
	if got := in.post("/settings/users/"+me+"/delete", nil).Status; got < 400 {
		t.Errorf("deleting the only administrator = %d, want a refusal", got)
	}
	if got := in.post("/settings/users/"+me+"/role", url.Values{"role": {"user"}}).Status; got < 400 {
		t.Errorf("demoting the only administrator = %d, want a refusal", got)
	}
	// Still an administrator, and still able to work.
	in.get("/settings/users").mustShow("Administrador")
}

// The bootstrap pair creates the administrator on a first run and resets the
// password on a later one. It is the documented way back in.
func TestBootstrapFromTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"TABERNACLE_DATA_DIR":       dir,
		"TABERNACLE_ADMIN_USER":     "jose",
		"TABERNACLE_ADMIN_PASSWORD": testPassword,
	}
	in := start(t, env)
	in.login("jose")
	in.get("/").mustContain(">Ajustes<")

	// A later boot with a new password resets it, and ends the old session.
	in.stop()
	in.env["TABERNACLE_ADMIN_PASSWORD"] = "una contraseña nueva"
	in.boot()

	in.get("/").mustContain(">Acceder<")
	if got := in.post("/login", url.Values{"user": {"jose"}, "password": {testPassword}}).Status; got != http.StatusUnauthorized {
		t.Errorf("the old password still works: %d", got)
	}
	in.login2("jose", "una contraseña nueva")
	in.get("/").mustContain(">Ajustes<")
}

// Read-only mode is about the collection.
func TestReadOnlyModeAndAccountsTogether(t *testing.T) {
	in := start(t, map[string]string{"TABERNACLE_READ_ONLY": "true"})
	in.writeSong("Traditional/Amazing Grace.song", amazingGrace)
	in.claim("jose")

	if got := in.post("/songs", url.Values{"text": {newSong}}).Status; got != http.StatusForbidden {
		t.Errorf("a song write in read-only mode = %d, want 403", got)
	}
	if got := in.post("/settings/profile", url.Values{"user": {"josedavid"}, "theme": {"dark"}}).Status; got != http.StatusSeeOther {
		t.Errorf("a profile edit in read-only mode = %d", got)
	}
	in.get("/").mustContain("Amazing Grace")
}

// One address prints the whole set, which is what a rehearsal needs.
func TestASongbookInOneAddress(t *testing.T) {
	in := start(t, nil)
	in.seed()

	in.get("/songbook").mustContain(`name="s" value="traditional-amazing-grace"`)

	book := in.get("/songbook/print?s=traditional-amazing-grace&s=otro-con-cejilla&s=prueba-jazz&index=1")
	if book.Status != http.StatusOK {
		t.Fatalf("status = %d", book.Status)
	}
	book.mustContain("Amazing Grace", "Con cejilla", "Jazz", "songbook-index")
	if n := strings.Count(book.Body, `class="songbook-song"`); n != 3 {
		t.Errorf("the book holds %d songs, want 3", n)
	}
	// It is for paper.
	book.mustNotContain(`class="top`, "chordcard")
}

func adminID(t *testing.T, in *instance) string {
	t.Helper()
	body := in.page("/settings/users")
	i := strings.Index(body, "/settings/users/")
	if i < 0 {
		t.Fatal("the accounts page names no account")
	}
	rest := body[i+len("/settings/users/"):]
	j := strings.IndexByte(rest, '/')
	if j < 0 {
		t.Fatal("could not read an account id out of the page")
	}
	return rest[:j]
}
