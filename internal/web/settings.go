package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jalmena/tabernacle/internal/account"
	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/i18n"
)

// themeClass maps a stored preference onto the class that carries it.
//
// A class rather than an inline style, because the content security policy has no
// 'unsafe-inline' for styles and an inline one would simply be dropped.
func themeClass(theme string) string {
	switch theme {
	case account.ThemeLight:
		return "theme-light"
	case account.ThemeDark:
		return "theme-dark"
	}
	return ""
}

// settingsBody is the settings page.
type settingsBody struct {
	Tabs []navLink

	// Claim is the form an unclaimed instance shows instead of a profile: there
	// is nobody to have one yet.
	Claim bool

	Name string
	// The three personalisation choices, each as its options so the template
	// only lays them out.
	Themes    []option
	Languages []option
	Notations []option
	IsAdmin   bool

	Problem string
}

// option is one choice in a group of radios.
type option struct {
	Value string
	Label string
	On    bool
}

// usersBody is the list of accounts.
type usersBody struct {
	Tabs         []navLink
	Users        []userRow
	Registration bool
	Problem      string
}

// userEditBody is one account, with everything that can be done to it.
//
// A page rather than a row of controls in a table: four actions per account is
// more than a row can hold without turning into something you have to scroll
// sideways to read.
type userEditBody struct {
	Tabs    []navLink
	User    userRow
	Problem string
}

type userRow struct {
	ID       string
	Name     string
	Admin    bool
	Self     bool
	Created  string
	LastSeen string
}

// settingsTabs is the settings page's own navigation.
//
// The library's vocabulary — Canciones, Artistas, Géneros — has no business on a
// settings page, so those templates leave the section row out entirely. This
// replaces it, because a page whose only way out is the brand link strands the
// reader.
func (s *Server) settingsTabs(r *http.Request, current string) []navLink {
	t := i18n.For(i18n.Lang(viewerOf(r).User.Lang))

	tabs := []navLink{{Label: t.T("settings.tab.account"), URL: "/settings", On: current == "profile"}}
	if viewerOf(r).MayAdmin() {
		tabs = append(tabs, navLink{Label: t.T("settings.tab.users"), URL: "/settings/users",
			On: current == "users"})
	}
	if !s.cfg.ReadOnly {
		tabs = append(tabs, navLink{Label: t.T("settings.tab.transfer"), URL: "/import",
			On: current == "import"})
	}
	tabs = append(tabs, navLink{Label: t.T("settings.tab.songbook"), URL: "/songbook",
		On: current == "songbook"})
	return tabs
}

// requireAccess gates a page that is not for the public, answering the way a
// guarded write does: signed out gets the login form, signed in without the
// privilege gets a refusal.
func (s *Server) requireAccess(w http.ResponseWriter, r *http.Request, need access) bool {
	if s.permitted(r, need) {
		return true
	}
	s.refusePrivilege(w, r, need)
	return false
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAccess(w, r, accessWriter) {
		return
	}
	s.renderSettings(w, r, http.StatusOK, "")
}

func (s *Server) renderSettings(w http.ResponseWriter, r *http.Request, status int, problem string) {
	v := viewerOf(r)
	p := s.page(r, "", nil)
	body := settingsBody{
		Tabs:    s.settingsTabs(r, "profile"),
		Claim:   !v.In,
		Name:    v.User.Name,
		IsAdmin: v.User.IsAdmin(),
		Problem: problem,
	}
	if !body.Claim {
		body.Themes = []option{
			{account.ThemeSystem, p.T("settings.theme.system"), v.User.Theme == account.ThemeSystem},
			{account.ThemeLight, p.T("settings.theme.light"), v.User.Theme == account.ThemeLight},
			{account.ThemeDark, p.T("settings.theme.dark"), v.User.Theme == account.ThemeDark},
		}
		// A language names itself, so somebody looking for their own recognises
		// it however the rest of the page reads.
		for _, l := range i18n.All() {
			body.Languages = append(body.Languages, option{
				Value: string(l), Label: l.Name(), On: string(l) == v.User.Lang,
			})
		}
		if v.User.Lang == "" {
			body.Languages[0].On = true
		}
		// Note names are a reading preference like the other two. What the
		// instance is configured with is only the answer for somebody who has
		// not chosen.
		current := v.User.Notation
		if current == "" {
			current = s.cfg.Notation
		}
		body.Notations = []option{
			{config.NotationEnglish, "A B C", current == config.NotationEnglish},
			{config.NotationSolfege, "Do Re Mi", current == config.NotationSolfege},
		}
	}

	s.render(w, r, status, "settings.html", s.page(r, p.T("settings.title"), body))
}

// handleClaim creates the first administrator of an instance that has none.
//
// Open on purpose, and only while there is nobody: this is how an existing
// collection gets its first account. It refuses the instant one exists, so it
// cannot be used to add a second.
func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	if s.acc.Count() != 0 {
		s.fail(w, r, http.StatusForbidden, "fail.claimed", "fail.claimed.detail")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("user"))
	password := r.PostFormValue("password")
	if password != r.PostFormValue("repeat") {
		s.renderSettings(w, r, http.StatusBadRequest, i18n.For(s.langFor(r)).T("register.mismatch"))
		return
	}

	u, err := s.acc.Create(name, password, account.RoleAdmin)
	if err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.log.Info("the instance was claimed", "user", u.Name)
	s.startSession(w, r, u)
	s.redirect(w, r, "/settings", "claimed")
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	if !v.In {
		s.refusePrivilege(w, r, accessWriter)
		return
	}
	if name := strings.TrimSpace(r.PostFormValue("user")); name != "" && name != v.User.Name {
		if err := s.acc.Rename(v.User.ID, name); err != nil {
			s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
			return
		}
	}
	s.redirect(w, r, "/settings", "profile")
}

// handleLook saves the three choices about how the interface reads.
//
// One form, because they are one decision from the reader's side: what this
// looks like to me.
func (s *Server) handleLook(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	if !v.In {
		s.refusePrivilege(w, r, accessWriter)
		return
	}
	if err := s.acc.SetTheme(v.User.ID, r.PostFormValue("theme")); err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	if err := s.acc.SetLang(v.User.ID, r.PostFormValue("lang")); err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	if err := s.acc.SetNotation(v.User.ID, r.PostFormValue("notation")); err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings", "language")
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	if !v.In {
		s.refusePrivilege(w, r, accessWriter)
		return
	}
	next := r.PostFormValue("password")
	if next != r.PostFormValue("repeat") {
		s.renderSettings(w, r, http.StatusBadRequest, i18n.For(s.langFor(r)).T("register.mismatch"))
		return
	}
	err := func() error {
		_, e := s.hashSlot(func() (account.User, error) {
			return account.User{}, s.acc.ChangePassword(v.User.ID, r.PostFormValue("current"), next)
		})
		return e
	}()
	if err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	// The epoch moved, so this device's own cookie is now stale too. Reissue it,
	// or changing a password would log you out of the browser you did it in.
	if u, ok := s.acc.ByID(v.User.ID); ok {
		s.startSession(w, r, u)
	}
	s.redirect(w, r, "/settings", "password")
}

// handleRevokeSessions is "log out everywhere", including a device somebody no
// longer has.
func (s *Server) handleRevokeSessions(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	if !v.In {
		s.refusePrivilege(w, r, accessWriter)
		return
	}
	if err := s.acc.BumpEpoch(v.User.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	// Except this one: the person pressing the button should not be signed out by
	// their own action.
	if u, ok := s.acc.ByID(v.User.ID); ok {
		s.startSession(w, r, u)
	}
	s.redirect(w, r, "/settings", "logged-out")
}

// --- accounts ---

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAccess(w, r, accessAdmin) {
		return
	}
	s.renderUsers(w, r, http.StatusOK, "")
}

func (s *Server) renderUsers(w http.ResponseWriter, r *http.Request, status int, problem string) {
	me := viewerOf(r).User.ID
	body := usersBody{
		Tabs:         s.settingsTabs(r, "users"),
		Registration: s.acc.RegistrationOpen(),
		Problem:      problem,
	}
	for _, u := range s.acc.List() {
		body.Users = append(body.Users, rowFor(u, me))
	}
	s.render(w, r, status, "users.html", s.page(r, "users.title", body))
}

// handleUserEdit is one account's own page.
func (s *Server) handleUserEdit(w http.ResponseWriter, r *http.Request) {
	if !s.requireAccess(w, r, accessAdmin) {
		return
	}
	s.renderUserEdit(w, r, http.StatusOK, "")
}

func (s *Server) renderUserEdit(w http.ResponseWriter, r *http.Request, status int, problem string) {
	u, ok := s.acc.ByID(r.PathValue("id"))
	if !ok {
		s.fail(w, r, http.StatusNotFound, "fail.nouser", "fail.nouser.detail")
		return
	}
	body := userEditBody{
		Tabs:    s.settingsTabs(r, "users"),
		User:    rowFor(u, viewerOf(r).User.ID),
		Problem: problem,
	}
	s.render(w, r, status, "useredit.html", s.page(r, u.Name, body))
}

// handleSetRegistration opens or closes self-service registration.
func (s *Server) handleSetRegistration(w http.ResponseWriter, r *http.Request) {
	if err := s.acc.SetRegistrationOpen(r.PostFormValue("open") == "1"); err != nil {
		s.serverError(w, r, err)
		return
	}
	code := "registration-closed"
	if s.acc.RegistrationOpen() {
		code = "registration-open"
	}
	s.redirect(w, r, "/settings/users", code)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	role := account.RoleUser
	if r.PostFormValue("role") == string(account.RoleAdmin) {
		role = account.RoleAdmin
	}
	_, err := s.hashSlot(func() (account.User, error) {
		return s.acc.Create(strings.TrimSpace(r.PostFormValue("user")), r.PostFormValue("password"), role)
	})
	if err != nil {
		s.renderUsers(w, r, http.StatusBadRequest, accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings/users", "user-created")
}

func (s *Server) handleSetRole(w http.ResponseWriter, r *http.Request) {
	role := account.RoleUser
	if r.PostFormValue("role") == string(account.RoleAdmin) {
		role = account.RoleAdmin
	}
	if err := s.acc.SetRole(viewerOf(r).User.ID, r.PathValue("id"), role); err != nil {
		s.renderUserEdit(w, r, statusFor(err), accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings/users/"+r.PathValue("id"), "role-changed")
}

func (s *Server) handleRenameUser(w http.ResponseWriter, r *http.Request) {
	if err := s.acc.Rename(r.PathValue("id"), strings.TrimSpace(r.PostFormValue("user"))); err != nil {
		s.renderUserEdit(w, r, statusFor(err), accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings/users/"+r.PathValue("id"), "profile")
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	_, err := s.hashSlot(func() (account.User, error) {
		return account.User{}, s.acc.SetPassword(r.PathValue("id"), r.PostFormValue("password"))
	})
	if err != nil {
		s.renderUserEdit(w, r, statusFor(err), accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings/users/"+r.PathValue("id"), "user-reset")
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.acc.Delete(viewerOf(r).User.ID, r.PathValue("id")); err != nil {
		s.renderUserEdit(w, r, statusFor(err), accountProblem(i18n.For(s.langFor(r)), err))
		return
	}
	s.redirect(w, r, "/settings/users", "user-deleted")
}

// accountProblem turns a domain error into something to show a person.
//
// The mapping is exhaustive over the errors the domain declares, and anything
// unrecognised becomes a generic sentence rather than leaking a Go error into the
// page.
func rowFor(u account.User, me string) userRow {
	row := userRow{
		ID:      u.ID,
		Name:    u.Name,
		Admin:   u.IsAdmin(),
		Self:    u.ID == me,
		Created: u.CreatedAt.Format("2006-01-02"),
	}
	if !u.LastLoginAt.IsZero() {
		row.LastSeen = u.LastLoginAt.Format("2006-01-02")
	}
	return row
}

// accountProblem turns a refusal from the accounts store into a sentence the
// reader can act on. The store speaks in error values, which is what lets it
// stay a package that knows nothing about pages or languages.
func accountProblem(say *i18n.Printer, err error) string {
	switch {
	case errors.Is(err, account.ErrNameTaken):
		return say.T("problem.nametaken")
	case errors.Is(err, account.ErrNameInvalid):
		return say.T("problem.nameinvalid")
	case errors.Is(err, account.ErrPasswordTooShort):
		return say.T("problem.passwordshort")
	case errors.Is(err, account.ErrBadCredentials):
		return say.T("problem.badpassword")
	case errors.Is(err, account.ErrThemeInvalid):
		return say.T("problem.notheme")
	case errors.Is(err, account.ErrLangInvalid):
		return say.T("problem.nolang")
	case errors.Is(err, account.ErrNotationInvalid):
		return say.T("problem.nonotation")
	case errors.Is(err, account.ErrLastAdmin):
		return say.T("problem.lastadmin")
	case errors.Is(err, account.ErrSelfTarget):
		return say.T("problem.self")
	case errors.Is(err, account.ErrNotFound):
		return say.T("problem.gone")
	}
	return say.T("problem.unsaved")
}

func statusFor(err error) int {
	if errors.Is(err, account.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
