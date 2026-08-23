package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jalmena/tabernacle/internal/account"
	"github.com/jalmena/tabernacle/internal/i18n"
)

// sessionCookie is where the signed token lives.
const sessionCookie = "tabernacle_session"

// sessionLife is how long a session lasts before it has to be renewed by logging
// in again. A songbook is not a bank; there is no idle timeout.
const sessionLife = 30 * 24 * time.Hour

// viewer is who is asking. Every request has one, even an anonymous one.
type viewer struct {
	User account.User
	// In is true when a valid session cookie was presented.
	In bool
	// Unclaimed is true when no account exists at all, which is how a collection
	// that predates accounts keeps working until somebody claims it.
	Unclaimed bool
}

// MayWrite reports whether this viewer may change the collection.
func (v viewer) MayWrite() bool { return v.In || v.Unclaimed }

// MayAdmin reports whether this viewer may manage accounts and settings.
//
// Unclaimed is deliberately absent: claiming an instance is its own route, with
// its own guard, and it is the only thing an anonymous visitor may do here.
func (v viewer) MayAdmin() bool { return v.In && v.User.IsAdmin() }

// viewerKey is the one context key in this package. A per-request user has to
// reach s.page, which is called from about fifteen places, and threading a
// parameter through all of them would be worse than this.
type viewerKey struct{}

// viewerOf reads the viewer the authenticate middleware established.
//
// A request that never went through that middleware gets the zero value, which
// can neither write nor administer. Failing closed is the only safe default for
// an accessor that cannot tell why it was not set.
func viewerOf(r *http.Request) viewer {
	if v, ok := r.Context().Value(viewerKey{}).(viewer); ok {
		return v
	}
	return viewer{}
}

// authenticate resolves the session cookie once per request.
//
// A cookie that is forged, expired, or made stale by a password change is treated
// as anonymous *and cleared*, so a browser does not spend a month retrying a
// token that will never work again.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := viewer{Unclaimed: s.acc.Count() == 0}

		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			if u, expires, ok := s.session(c.Value); ok {
				v.User, v.In = u, true
				// Renew once the session is more than half spent, so somebody who
				// uses the songbook every week never has to log in again.
				if time.Until(expires) < sessionLife/2 {
					s.startSession(w, r, u)
				}
			} else {
				s.endSession(w, r)
			}
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), viewerKey{}, v)))
	})
}

// session resolves a token to the account it names.
//
// The epoch has to match what the file says, which is what makes a password
// change and "log out everywhere" take effect on every device at once.
func (s *Server) session(token string) (account.User, time.Time, bool) {
	c, err := account.Parse(s.acc.Secret(), token, time.Now())
	if err != nil {
		return account.User{}, time.Time{}, false
	}
	u, ok := s.acc.ByID(c.ID)
	if !ok || u.Epoch != c.Epoch {
		return account.User{}, time.Time{}, false
	}
	return u, c.Expires, true
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u account.User) {
	expires := time.Now().Add(sessionLife)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    account.Mint(s.acc.Secret(), u, expires),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(sessionLife.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.overTLS(r),
	})
}

func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.overTLS(r),
	})
}

// overTLS decides whether the cookie may be marked Secure.
//
// It cannot be unconditional: the ordinary deployment is plain HTTP on a home
// network, and a Secure cookie would never be sent, producing a login form that
// appears to do nothing. X-Forwarded-Proto is not consulted, because nothing else
// in this application trusts a header a hop can invent; the configured base URL
// is the honest signal.
func (s *Server) overTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.cfg.BaseURL != nil && s.cfg.BaseURL.Scheme == "https"
}

// --- the login pages ---

type loginBody struct {
	Next string
	User string
	// Registration is whether to offer the way to make an account, which is off
	// unless an administrator has opened it.
	Registration bool
	Problem      string
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if viewerOf(r).In {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	body := loginBody{
		Next:         safeNext(r.URL.Query().Get("next")),
		Registration: s.acc.RegistrationOpen(),
	}
	s.render(w, r, http.StatusOK, "login.html", s.page(r, "login.title", body))
}

// handleLogin checks a name and password.
//
// Every failure reads the same, whatever it was. Distinguishing "no such user"
// from "wrong password" tells an attacker which names are worth attacking, and
// account.Authenticate already spends the same work on both.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	say := i18n.For(s.langFor(r))
	name := strings.TrimSpace(r.PostFormValue("user"))
	password := r.PostFormValue("password")
	next := safeNext(r.PostFormValue("next"))

	refuse := func(status int, problem string) {
		body := loginBody{
			Next:         next,
			User:         name,
			Registration: s.acc.RegistrationOpen(),
			Problem:      problem,
		}
		s.render(w, r, status, "login.html", s.page(r, "login.title", body))
	}

	if !s.logins.allowed(strings.ToLower(name)) || !s.logins.allowed(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		refuse(http.StatusTooManyRequests, say.T("login.throttled"))
		return
	}

	u, err := s.hashSlot(func() (account.User, error) { return s.acc.Authenticate(name, password) })
	if err != nil {
		s.logins.spend(strings.ToLower(name))
		s.logins.spend(clientIP(r))
		s.log.Warn("refused a login", "remote", clientIP(r))
		refuse(http.StatusUnauthorized, say.T("login.wrong"))
		return
	}

	s.startSession(w, r, u)
	s.redirect(w, r, next, "welcome")
}

// handleRegisterForm offers to make an account, when an administrator has opened
// the door.
//
// A 404 when it is closed rather than a 403: a route that is not open is not a
// route, and saying "you may not" invites somebody to keep trying.
func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if !s.acc.RegistrationOpen() {
		s.handleNotFound(w, r)
		return
	}
	if viewerOf(r).In {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "register.html",
		s.page(r, "register.title", loginBody{Registration: true}))
}

// handleRegister creates an ordinary account.
//
// Never an administrator: opening registration lets people add songs, not decide
// who else may.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.acc.RegistrationOpen() {
		s.handleNotFound(w, r)
		return
	}
	say := i18n.For(s.langFor(r))
	name := strings.TrimSpace(r.PostFormValue("user"))
	password := r.PostFormValue("password")

	refuse := func(problem string) {
		s.render(w, r, http.StatusBadRequest, "register.html",
			s.page(r, "register.title", loginBody{User: name, Registration: true, Problem: problem}))
	}
	if password != r.PostFormValue("repeat") {
		refuse(say.T("register.mismatch"))
		return
	}
	// Rate limited like a login: the work is the same, and so is the lever.
	if !s.logins.allowed(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		refuse(say.T("login.throttled"))
		return
	}
	u, err := s.hashSlot(func() (account.User, error) {
		return s.acc.Create(name, password, account.RoleUser)
	})
	if err != nil {
		s.logins.spend(clientIP(r))
		refuse(accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.log.Info("a visitor created an account", "user", u.Name)
	s.startSession(w, r, u)
	s.redirect(w, r, "/", "registered")
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.endSession(w, r)
	s.redirect(w, r, "/", "bye")
}

// hashSlot bounds how many passwords are hashed at once.
//
// Hashing is expensive on purpose, and it is reachable without a session, so a
// handful of simultaneous attempts could otherwise occupy every core of a small
// machine. Attempts queue instead.
func (s *Server) hashSlot(fn func() (account.User, error)) (account.User, error) {
	s.hashing <- struct{}{}
	defer func() { <-s.hashing }()
	return fn()
}

// safeNext keeps a redirect target on this site.
//
// Without it the login form is an open redirector: somebody sends a link to our
// own login page with ?next=https://evil.example, and the reader arrives
// somewhere else having just typed their password.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.ContainsAny(next, `\:`) {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return next
}

// --- refusing too many attempts ---

// Login limits. Ten attempts is more than a person needs and far fewer than a
// guesser wants, and a token a minute means a forgotten password is a short wait
// rather than a lockout.
const (
	loginBurst      = 10
	loginRefill     = time.Minute
	limiterCapacity = 1024
)

// limiter is a token bucket per key, in memory.
//
// Losing the counters on a restart is the right trade for a household server: no
// table, no dependency, and an attacker cannot cause the restart that would clear
// them.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func newLimiter() *limiter {
	return &limiter{buckets: make(map[string]*bucket), now: time.Now}
}

// allowed reports whether a key has budget left, without spending any. Only a
// failed attempt costs, so somebody logging in on three devices is not punished
// for it.
func (l *limiter) allowed(key string) bool {
	if key == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refill(key).tokens >= 1
}

// spend charges one token for a failure.
func (l *limiter) spend(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refill(key)
	if b.tokens >= 1 {
		b.tokens--
	}
}

// refill brings a bucket up to date, creating it if needed. Callers hold the
// lock.
func (l *limiter) refill(key string) *bucket {
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= limiterCapacity {
			l.evict(now)
		}
		b = &bucket{tokens: loginBurst, seen: now}
		l.buckets[key] = b
	}
	b.tokens = min(loginBurst, b.tokens+now.Sub(b.seen).Seconds()/loginRefill.Seconds())
	b.seen = now
	return b
}

// evict drops buckets that have refilled completely, since they are
// indistinguishable from a key that has never been seen.
func (l *limiter) evict(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.seen).Seconds()/loginRefill.Seconds() >= loginBurst {
			delete(l.buckets, k)
		}
	}
	// Still full: a flood of distinct keys. Start again rather than grow without
	// bound; the cost is that the flood gets a fresh budget, which it would get
	// from a restart anyway.
	if len(l.buckets) >= limiterCapacity {
		l.buckets = make(map[string]*bucket)
	}
}

var errNotSignedIn = errors.New("web: not signed in")
