package web

import (
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jalmena/tabernacle/internal/account"
)

// An instance with no accounts offers to be claimed, and claiming it signs the
// claimant in as its administrator.
func TestClaimingAnInstance(t *testing.T) {
	ts := newTestServer(t, basicSongs())

	page := ts.get(t, "/settings").Body.String()
	mustHave(t, page, `action="/settings/claim"`, "Reclamar esta instancia")
	// No settings tabs yet: there is nothing behind them.
	mustNotHave(t, page, `href="/settings/users"`)

	w := ts.postAnon(t, "/settings/claim", url.Values{
		"user": {"jose"}, "password": {testPassword}, "repeat": {testPassword},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("claiming = %d\n%s", w.Code, w.Body.String())
	}
	if ts.acc.Count() != 1 {
		t.Fatalf("the instance holds %d accounts", ts.acc.Count())
	}
	if !ts.acc.List()[0].IsAdmin() {
		t.Error("the first account is not an administrator")
	}
	// And the claimant is signed in, or they would have to log in immediately.
	if sessionOf(t, w) == nil {
		t.Error("claiming did not sign anybody in")
	}
}

// Claiming works once. It is open because it has to be, so it closes the instant
// there is somebody to ask.
func TestAClaimedInstanceCannotBeClaimedAgain(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "jose", account.RoleAdmin)

	w := ts.postAnon(t, "/settings/claim", url.Values{
		"user": {"intruso"}, "password": {testPassword}, "repeat": {testPassword},
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if ts.acc.Count() != 1 {
		t.Errorf("the instance now holds %d accounts", ts.acc.Count())
	}
}

func TestClaimingRefusesMismatchedPasswords(t *testing.T) {
	ts := newTestServer(t, basicSongs())

	w := ts.postAnon(t, "/settings/claim", url.Values{
		"user": {"jose"}, "password": {testPassword}, "repeat": {"otra cosa larga"},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no coinciden") {
		t.Error("the refusal does not say what was wrong")
	}
	if ts.acc.Count() != 0 {
		t.Error("an account was created anyway")
	}
}

// Renaming keeps the session, because the cookie carries an id.
func TestRenamingYourselfKeepsYouSignedIn(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	w := ts.post(t, "/settings/profile", url.Values{"user": {"josedavid"}, "theme": {""}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d\n%s", w.Code, w.Body.String())
	}
	body := ts.get(t, "/settings").Body.String()
	mustHave(t, body, `value="josedavid"`, ">Ajustes<")
}

func TestProfileRefusesAnUnusableName(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	w := ts.post(t, "/settings/profile", url.Values{"user": {"jo se"}, "theme": {""}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	mustHave(t, w.Body.String(), "letras, números")
	if got := ts.acc.List()[0].Name; got != "jose" {
		t.Errorf("the name changed to %q anyway", got)
	}
}

// A password change needs the current one: an unlocked laptop should not become a
// permanent takeover.
func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	w := ts.post(t, "/settings/password", url.Values{
		"current": {"la que no es"}, "password": {"otra contraseña larga"}, "repeat": {"otra contraseña larga"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	mustHave(t, w.Body.String(), "contraseña actual no es correcta")
}

// Only an administrator manages accounts, and a plain user cannot get there by
// changing a form's action.
func TestAPlainUserCannotManageAccounts(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "admin", account.RoleAdmin)
	ts.as(t, "ana", account.RoleUser)

	if got := ts.get(t, "/settings/users").Code; got != http.StatusForbidden {
		t.Errorf("GET /settings/users = %d for a plain user, want 403", got)
	}
	// And the settings page does not offer the door.
	mustNotHave(t, ts.get(t, "/settings").Body.String(), `href="/settings/users"`)

	before := ts.acc.Count()
	if got := ts.post(t, "/settings/users", url.Values{
		"user": {"colado"}, "password": {testPassword}, "role": {"admin"},
	}).Code; got != http.StatusForbidden {
		t.Errorf("POST /settings/users = %d, want 403", got)
	}
	if ts.acc.Count() != before {
		t.Error("a plain user created an account")
	}
}

func TestAnAdministratorManagesAccounts(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	if w := ts.post(t, "/settings/users", url.Values{
		"user": {"ana"}, "password": {testPassword}, "role": {"user"},
	}); w.Code != http.StatusSeeOther {
		t.Fatalf("creating an account = %d\n%s", w.Code, w.Body.String())
	}
	ana := findUser(t, ts, "ana")
	if ana.IsAdmin() {
		t.Error("ana was created as an administrator")
	}

	if w := ts.post(t, "/settings/users/"+ana.ID+"/role", url.Values{"role": {"admin"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("promoting = %d", w.Code)
	}
	if !findUser(t, ts, "ana").IsAdmin() {
		t.Error("ana was not promoted")
	}

	if w := ts.post(t, "/settings/users/"+ana.ID+"/delete", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("deleting = %d", w.Code)
	}
	if ts.acc.Count() != 1 {
		t.Errorf("%d accounts remain, want 1", ts.acc.Count())
	}
}

// The two rules that make locking yourself out impossible, through the HTTP layer.
func TestTheLastAdministratorIsProtectedThroughTheWeb(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	me := ts.as(t, "jose", account.RoleAdmin)

	w := ts.post(t, "/settings/users/"+me.ID+"/delete", nil)
	if w.Code < 400 {
		t.Fatalf("deleting yourself = %d, want a refusal", w.Code)
	}
	mustHave(t, w.Body.String(), "tu propia cuenta")

	w = ts.post(t, "/settings/users/"+me.ID+"/role", url.Values{"role": {"user"}})
	if w.Code < 400 {
		t.Fatalf("demoting yourself = %d, want a refusal", w.Code)
	}
	if !findUser(t, ts, "jose").IsAdmin() {
		t.Error("the last administrator was demoted")
	}
}

// An administrator's reset logs that person out everywhere, which is the point of
// a reset.
func TestAnAdministratorsResetEndsTheOtherSessions(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "admin", account.RoleAdmin)
	ts.as(t, "ana", account.RoleUser)
	anaSession := ts.session
	ana := findUser(t, ts, "ana")

	ts.login(t, "admin")
	if w := ts.post(t, "/settings/users/"+ana.ID+"/password", url.Values{
		"password": {"una contraseña nueva"},
	}); w.Code != http.StatusSeeOther {
		t.Fatalf("resetting = %d\n%s", w.Code, w.Body.String())
	}

	ts.session = anaSession
	if strings.Contains(ts.get(t, "/").Body.String(), ">Ajustes<") {
		t.Error("ana is still signed in after her password was reset")
	}
}

// Note names are a reading preference, so they belong to the person and follow
// them from page to page.
func TestNoteNamesAreAPersonalChoice(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	if w := ts.post(t, "/settings/look", url.Values{
		"lang": {""}, "theme": {""}, "notation": {"solfege"},
	}); w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d\n%s", w.Code, w.Body.String())
	}
	// Every song now reads in solfege, without asking per page.
	mustHave(t, ts.get(t, "/songs/traditional-amazing-grace").Body.String(), "Sol")
	// And a stranger still gets the instance's own default.
	mustNotHave(t, ts.getAnon(t, "/songs/traditional-amazing-grace").Body.String(), ">Sol<")
}

func TestAnUnknownNoteNamingIsRefused(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	if got := ts.post(t, "/settings/look", url.Values{
		"lang": {""}, "theme": {""}, "notation": {"klingon"},
	}).Code; got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

// Read-only mode governs the collection, not the application.
func TestReadOnlyModeStillLetsYouManageAccounts(t *testing.T) {
	ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
		"TABERNACLE_READ_ONLY": "true",
	})
	ts.as(t, "jose", account.RoleAdmin)

	if got := ts.post(t, "/settings/profile", url.Values{"user": {"josedavid"}, "theme": {"dark"}}).Code; got != http.StatusSeeOther {
		t.Errorf("editing a profile in read-only mode = %d", got)
	}
	if got := ts.post(t, "/settings/users", url.Values{
		"user": {"ana"}, "password": {testPassword}, "role": {"user"},
	}).Code; got != http.StatusSeeOther {
		t.Errorf("creating an account in read-only mode = %d", got)
	}
	// But songs are still refused.
	if got := ts.post(t, "/reindex", nil).Code; got != http.StatusForbidden {
		t.Errorf("a collection write in read-only mode = %d, want 403", got)
	}
}

// Pages are specific to whoever is asking now, so nothing in between may keep
// them.
func TestPagesAreNotCacheable(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	for _, path := range []string{"/", "/settings", "/login", "/songs/traditional-amazing-grace", "/nope"} {
		if got := ts.get(t, path).Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s sent Cache-Control %q, want no-store", path, got)
		}
	}
	// The stylesheet is still cached hard: it is the same for everybody and its
	// address changes when it does.
	body := ts.get(t, "/").Body.String()
	if m := regexp.MustCompile(`/static/[^"']+\.css`).FindString(body); m != "" {
		if got := ts.get(t, m).Header().Get("Cache-Control"); !strings.Contains(got, "max-age") {
			t.Errorf("the stylesheet is no longer cacheable: %q", got)
		}
	}
}

// A chosen theme has to be declared in every block that declares the palette, or
// switching to it would leave some colours behind.
func TestEveryThemeDeclaresTheWholePalette(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	blocks := map[string]string{
		":root":        `(?ms)^:root \{(.*?)^\}`,
		"prefers-dark": `(?ms)@media \(prefers-color-scheme: dark\) \{(.*?)^\}`,
		"theme-light":  `(?ms)^body\.theme-light \{(.*?)^\}`,
		"theme-dark":   `(?ms)^body\.theme-dark \{(.*?)^\}`,
	}
	property := regexp.MustCompile(`(--[a-z-]+)\s*:`)

	found := map[string][]string{}
	for name, pattern := range blocks {
		m := regexp.MustCompile(pattern).FindStringSubmatch(string(css))
		if m == nil {
			t.Fatalf("app.css has no %s block; this test cannot check anything", name)
		}
		var names []string
		for _, p := range property.FindAllStringSubmatch(m[1], -1) {
			names = append(names, p[1])
		}
		sort.Strings(names)
		found[name] = slices.Compact(names)
	}

	// :root also holds the sizes, which are not part of a palette.
	want := []string{}
	for _, n := range found[":root"] {
		if n != "--sheet-width" && n != "--radius" && n != "--song-size" {
			want = append(want, n)
		}
	}
	for _, name := range []string{"prefers-dark", "theme-light", "theme-dark"} {
		for _, token := range want {
			if !slices.Contains(found[name], token) {
				t.Errorf("%s does not declare %s", name, token)
			}
		}
	}
}

func findUser(t *testing.T, ts *testServer, name string) account.User {
	t.Helper()
	for _, u := range ts.acc.List() {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("there is no account called %q", name)
	return account.User{}
}
