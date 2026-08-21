package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jalmena/tabernacle/internal/account"
)

// writeRoute is a POST route and the least privilege that may reach it.
type writeRoute struct {
	path string
	need access
	form url.Values
}

// writeRoutes is every route that changes something, with what it demands.
//
// This table is the mechanised form of the promise in guarded's doc comment. A
// companion test reads the routing table out of the source and fails if a POST
// route is missing here, so a new write cannot be added without somebody deciding
// what it needs.
func writeRoutes() []writeRoute {
	const slug = "traditional-amazing-grace"
	return []writeRoute{
		{"/login", accessPublic, url.Values{"user": {"nobody"}, "password": {"x"}}},
		{"/logout", accessPublic, nil},
		{"/settings/claim", accessPublic, url.Values{"user": {"nuevo"}, "password": {testPassword}, "repeat": {testPassword}}},

		{"/songs", accessWriter, url.Values{"text": {pasted}}},
		{"/songs/new", accessWriter, nil},
		{"/songs/" + slug, accessWriter, url.Values{"text": {graceSong}, "hash": {"x"}}},
		{"/songs/" + slug + "/delete", accessWriter, nil},
		{"/songs/" + slug + "/transpose", accessWriter, url.Values{"t": {"2"}}},
		{"/songs/" + slug + "/commit-transpose", accessWriter, url.Values{"t": {"2"}}},
		{"/reindex", accessWriter, nil},
		{"/import", accessWriter, nil},
		{"/settings/profile", accessWriter, url.Values{"user": {"jose"}, "theme": {""}}},
		{"/settings/password", accessWriter, url.Values{"current": {testPassword}, "password": {"otra contraseña larga"}, "repeat": {"otra contraseña larga"}}},
		{"/settings/sessions/revoke", accessWriter, nil},

		{"/settings/users", accessAdmin, url.Values{"user": {"otro"}, "password": {testPassword}, "role": {"user"}}},
		{"/settings/users/xyz/role", accessAdmin, url.Values{"role": {"admin"}}},
		{"/settings/users/xyz/password", accessAdmin, url.Values{"password": {testPassword}}},
		{"/settings/users/xyz/delete", accessAdmin, nil},
		{"/settings/notation", accessAdmin, url.Values{"notation": {"solfege"}}},
	}
}

var postRoutePattern = regexp.MustCompile(`mux\.HandleFunc\("POST ([^"]+)"`)

// Every POST route the server registers has to appear in the table above, so that
// adding a write without deciding its privilege is a test failure rather than an
// oversight.
func TestEveryWriteRouteHasADeclaredPrivilege(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, r := range writeRoutes() {
		declared[shape(r.path)] = true
	}

	found := 0
	for _, m := range postRoutePattern.FindAllStringSubmatch(string(src), -1) {
		found++
		if !declared[m[1]] {
			t.Errorf("POST %s is registered but has no declared privilege in writeRoutes", m[1])
		}
	}
	if found == 0 {
		t.Fatal("no POST routes were found in server.go; this test proves nothing")
	}
	if found != len(writeRoutes()) {
		t.Errorf("server.go registers %d POST routes and the table lists %d",
			found, len(writeRoutes()))
	}
}

// shape turns a concrete path back into the pattern it was registered as, so the
// table can hold real paths and still be compared against the routing table.
func shape(path string) string {
	switch {
	case strings.HasPrefix(path, "/settings/users/") && strings.HasSuffix(path, "/role"):
		return "/settings/users/{id}/role"
	case strings.HasPrefix(path, "/settings/users/") && strings.HasSuffix(path, "/password"):
		return "/settings/users/{id}/password"
	case strings.HasPrefix(path, "/settings/users/") && strings.HasSuffix(path, "/delete"):
		return "/settings/users/{id}/delete"
	case path == "/songs/new" || path == "/songs":
		return path
	case strings.HasPrefix(path, "/songs/") && strings.HasSuffix(path, "/delete"):
		return "/songs/{slug}/delete"
	case strings.HasPrefix(path, "/songs/") && strings.HasSuffix(path, "/transpose"):
		return "/songs/{slug}/transpose"
	case strings.HasPrefix(path, "/songs/") && strings.HasSuffix(path, "/commit-transpose"):
		return "/songs/{slug}/commit-transpose"
	case strings.HasPrefix(path, "/songs/"):
		return "/songs/{slug}"
	}
	return path
}

// The table, driven three times. This is the test that would catch a privilege
// escalation, so it asks about every route from every standing.
func TestWriteRoutesEnforceTheirPrivilege(t *testing.T) {
	for _, rt := range writeRoutes() {
		t.Run("anonymous "+rt.path, func(t *testing.T) {
			ts := newTestServer(t, basicSongs())
			// Claimed, so that unclaimed mode is not what lets a stranger in.
			ts.account(t, "admin", account.RoleAdmin)

			w := ts.postAnon(t, rt.path, rt.form)
			if rt.need == accessPublic {
				if why, blocked := blockedBy(w); blocked {
					t.Errorf("POST %s refused an anonymous request for %s", rt.path, why)
				}
				return
			}
			if why, blocked := blockedBy(w); !blocked || why != "being signed out" {
				t.Errorf("POST %s = %d (%s) for a stranger, want the login page",
					rt.path, w.Code, why)
			}
		})

		t.Run("user "+rt.path, func(t *testing.T) {
			ts := newTestServer(t, basicSongs())
			ts.account(t, "admin", account.RoleAdmin)
			ts.as(t, "ana", account.RoleUser)

			w := ts.post(t, rt.path, rt.form)
			why, blocked := blockedBy(w)
			if rt.need == accessAdmin {
				if !blocked || why != "not being an administrator" {
					t.Errorf("POST %s = %d (%s) for a plain user, want a refusal",
						rt.path, w.Code, why)
				}
				return
			}
			if blocked {
				t.Errorf("POST %s refused a signed-in user for %s", rt.path, why)
			}
		})

		t.Run("admin "+rt.path, func(t *testing.T) {
			ts := newTestServer(t, basicSongs())
			ts.as(t, "admin", account.RoleAdmin)

			if why, blocked := blockedBy(ts.post(t, rt.path, rt.form)); blocked {
				t.Errorf("POST %s refused an administrator for %s", rt.path, why)
			}
		})
	}
}

// A signed-out write gets the login page, not a dead end: the next thing to do is
// visible.
func TestASignedOutWriteOffersTheLoginForm(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "admin", account.RoleAdmin)

	w := ts.postAnon(t, "/songs", url.Values{"text": {pasted}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`action="/login"`, `name="password"`, "Inicia sesión"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refusal does not contain %q", want)
		}
	}
	// And nothing was written.
	if got := songCount(t, ts); got != 1 {
		t.Errorf("the collection holds %d songs, want 1", got)
	}
}

// Read stays public. That is the whole shape of the thing: anybody may read, a
// signed-in account may write.
func TestReadingStaysPublic(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "admin", account.RoleAdmin)

	for _, path := range []string{
		"/", "/artists", "/genres", "/?q=grace", "/export",
		"/songs/traditional-amazing-grace",
		"/songs/traditional-amazing-grace/print",
		"/songs/traditional-amazing-grace/raw",
		"/login", "/healthz", "/robots.txt",
	} {
		if got := ts.getAnon(t, path).Code; got != http.StatusOK {
			t.Errorf("GET %s = %d for a stranger, want 200", path, got)
		}
	}
}

// The header is where a reader finds out where they stand.
func TestTheHeaderSaysWhoYouAre(t *testing.T) {
	t.Run("unclaimed", func(t *testing.T) {
		ts := newTestServer(t, basicSongs())
		body := ts.get(t, "/").Body.String()
		mustHave(t, body, ">Acceder<", `href="/login"`, `class="banner`, `href="/songs/new"`)
	})
	t.Run("signed out of a claimed instance", func(t *testing.T) {
		ts := newTestServer(t, basicSongs())
		ts.account(t, "admin", account.RoleAdmin)
		body := ts.get(t, "/").Body.String()
		mustHave(t, body, ">Acceder<")
		mustNotHave(t, body, `href="/songs/new"`, `class="banner`)
	})
	t.Run("signed in", func(t *testing.T) {
		ts := newTestServer(t, basicSongs())
		ts.as(t, "jose", account.RoleUser)
		body := ts.get(t, "/").Body.String()
		mustHave(t, body, ">Ajustes<", `href="/settings"`, `href="/songs/new"`)
		mustNotHave(t, body, ">Acceder<", `class="banner`)
	})
	t.Run("read-only", func(t *testing.T) {
		ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
			"TABERNACLE_READ_ONLY": "true",
		})
		ts.as(t, "jose", account.RoleAdmin)
		body := ts.get(t, "/").Body.String()
		mustHave(t, body, ">Ajustes<")
		mustNotHave(t, body, `href="/songs/new"`)
	})
}

// The session cookie's attributes are the difference between a session and a
// liability.
func TestTheSessionCookieIsLockedDown(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "jose", account.RoleAdmin)

	w := ts.postAnon(t, "/login", url.Values{"user": {"jose"}, "password": {testPassword}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login = %d", w.Code)
	}
	c := sessionOf(t, w)
	if !c.HttpOnly {
		t.Error("the cookie is readable from script")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.Domain != "" {
		t.Errorf("Domain = %q, want host-only", c.Domain)
	}
	// Plain HTTP on a LAN is the ordinary deployment, and a Secure cookie would
	// simply never be sent.
	if c.Secure {
		t.Error("the cookie is Secure on a plain-HTTP instance, so it would never be sent")
	}
}

func TestTheCookieIsSecureBehindTLS(t *testing.T) {
	ts := newTestServerWithEnv(t, basicSongs(), t.TempDir(), map[string]string{
		"TABERNACLE_BASE_URL":      "https://songs.example.com",
		"TABERNACLE_ALLOWED_HOSTS": "songs.example.com,localhost",
	})
	ts.account(t, "jose", account.RoleAdmin)

	w := ts.postAnon(t, "/login", url.Values{"user": {"jose"}, "password": {testPassword}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login = %d\n%s", w.Code, w.Body.String())
	}
	if !sessionOf(t, w).Secure {
		t.Error("the cookie is not Secure although the base URL is https")
	}
}

// A cookie we did not issue, or one made stale by a password change, is refused
// and cleared: a browser should not spend a month retrying a dead token.
func TestABadCookieIsRefusedAndCleared(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	u := ts.as(t, "jose", account.RoleAdmin)
	good := ts.session

	tests := map[string]string{
		"forged":       sessionCookie + "=" + "abcdef.ghijkl",
		"empty parts":  sessionCookie + "=.",
		"another id":   sessionCookie + "=" + account.Mint(ts.acc.Secret(), account.User{ID: "deadbeefdeadbeef", Epoch: 1}, farFuture()),
		"stale epoch":  sessionCookie + "=" + account.Mint(ts.acc.Secret(), account.User{ID: u.ID, Epoch: u.Epoch + 5}, farFuture()),
		"other secret": sessionCookie + "=" + account.Mint([]byte(strings.Repeat("x", 32)), account.User{ID: u.ID, Epoch: u.Epoch}, farFuture()),
	}
	for name, cookie := range tests {
		t.Run(name, func(t *testing.T) {
			ts.session = cookie
			defer func() { ts.session = good }()

			w := ts.get(t, "/")
			if strings.Contains(w.Body.String(), ">Ajustes<") {
				t.Error("the bad cookie was accepted")
			}
			cleared := false
			for _, c := range w.Result().Cookies() {
				if c.Name == sessionCookie && c.MaxAge < 0 {
					cleared = true
				}
			}
			if !cleared {
				t.Error("the bad cookie was not cleared, so the browser will keep sending it")
			}
		})
	}

	// And the good one still works, so the test above is not passing by accident.
	ts.session = good
	if !strings.Contains(ts.get(t, "/").Body.String(), ">Ajustes<") {
		t.Error("the valid session stopped working")
	}
}

// Changing a password logs out every other device.
func TestChangingAPasswordEndsOtherSessions(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)
	phone := ts.session

	w := ts.post(t, "/settings/password", url.Values{
		"current":  {testPassword},
		"password": {"otra contraseña larga"},
		"repeat":   {"otra contraseña larga"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("changing the password = %d\n%s", w.Code, w.Body.String())
	}
	// This device keeps working: nobody should be signed out by their own action.
	ts.session = sessionOf(t, w).Name + "=" + sessionOf(t, w).Value
	if !strings.Contains(ts.get(t, "/").Body.String(), ">Ajustes<") {
		t.Error("the device that changed the password was signed out")
	}
	// The other one does not.
	ts.session = phone
	if strings.Contains(ts.get(t, "/").Body.String(), ">Ajustes<") {
		t.Error("the other device is still signed in")
	}
}

// Too many failures stop costing anything, because hashing is expensive on
// purpose and reachable without a session.
func TestRepeatedFailuresAreRefused(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "jose", account.RoleAdmin)

	for i := range loginBurst {
		if got := ts.postAnon(t, "/login", url.Values{"user": {"jose"}, "password": {"mala"}}).Code; got != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, got)
		}
	}
	w := ts.postAnon(t, "/login", url.Values{"user": {"jose"}, "password": {"mala"}})
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("the attempt past the budget = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("a 429 with no Retry-After")
	}
}

// A successful login does not spend budget, so somebody signing in on three
// devices is not punished for it.
func TestSuccessDoesNotSpendTheBudget(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.account(t, "jose", account.RoleAdmin)

	for i := range loginBurst + 5 {
		if got := ts.postAnon(t, "/login", url.Values{"user": {"jose"}, "password": {testPassword}}).Code; got != http.StatusSeeOther {
			t.Fatalf("login %d = %d", i+1, got)
		}
	}
}

// Without this the login form is an open redirector: a link to our own login page
// that lands the reader somewhere else having just typed their password.
func TestSafeNext(t *testing.T) {
	tests := map[string]string{
		"":                        "/",
		"/":                       "/",
		"/songs/x":                "/songs/x",
		"/settings/users":         "/settings/users",
		"//evil.example":          "/",
		"///evil.example":         "/",
		"https://evil.example":    "/",
		"http://evil.example":     "/",
		`/\evil.example`:          "/",
		"javascript:alert(1)":     "/",
		"//evil.example/settings": "/",
		"evil.example":            "/",
	}
	for in, want := range tests {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

// The settings pages leave the library's vocabulary out, which is the whole point
// of them having their own row.
func TestTheSettingsPagesCarryNoSectionRow(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	ts.as(t, "jose", account.RoleAdmin)

	for _, path := range []string{"/settings", "/settings/users"} {
		t.Run(path, func(t *testing.T) {
			body := ts.get(t, path).Body.String()
			if got := labelsIn(t, navBlock, body); len(got) != 0 {
				t.Errorf("the section row is present: %v", got)
			}
			mustNotHave(t, body, `href="/artists"`, `href="/genres"`)
			// But there is a way out.
			mustHave(t, body, `href="/settings"`, "Cerrar sesión")
		})
	}
}

// A chosen theme reaches <body>, where the custom properties everything else
// inherits are declared.
func TestAChosenThemeReachesTheBody(t *testing.T) {
	ts := newTestServer(t, basicSongs())
	u := ts.as(t, "jose", account.RoleAdmin)

	if err := ts.acc.SetTheme(u.ID, account.ThemeDark); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/settings", "/artists", "/songs/traditional-amazing-grace"} {
		if !strings.Contains(ts.get(t, path).Body.String(), `<body class="theme-dark">`) {
			t.Errorf("%s does not carry the chosen theme", path)
		}
	}
	// A stranger keeps their system preference, so no class at all.
	if !strings.Contains(ts.getAnon(t, "/").Body.String(), "<body>") {
		t.Error("an anonymous reader was given a theme class")
	}
}

// blockedBy reports whether a response is the privilege guard refusing, and which
// of its two refusals it is.
//
// Status alone cannot answer this: a login with the wrong password is a perfectly
// correct 401, and claiming an instance that already has accounts is a perfectly
// correct 403. What the guard produces is one of two specific pages, so that is
// what this looks for.
func blockedBy(w *httptest.ResponseRecorder) (string, bool) {
	body := w.Body.String()
	switch {
	case w.Code == http.StatusUnauthorized && strings.Contains(body, "Inicia sesión para guardar cambios"):
		return "being signed out", true
	case w.Code == http.StatusForbidden && strings.Contains(body, "Esta acción es solo para administradores"):
		return "not being an administrator", true
	case w.Code == http.StatusForbidden && strings.Contains(body, "Solo lectura"):
		return "read-only mode", true
	}
	return "", false
}

// --- helpers ---

func mustHave(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("the page does not contain %q", w)
		}
	}
}

func mustNotHave(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("the page contains %q and should not", u)
		}
	}
}

func sessionOf(t *testing.T, w interface{ Result() *http.Response }) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie was set")
	return nil
}

// farFuture is an expiry no test will outlive, for tokens that are meant to be
// refused for some other reason.
func farFuture() time.Time { return time.Now().Add(24 * time.Hour) }

func songCount(t *testing.T, ts *testServer) int {
	t.Helper()
	st, err := ts.lib.Stats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return st.Songs
}
