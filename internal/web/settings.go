package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jalmena/tabernacle/internal/account"
	"github.com/jalmena/tabernacle/internal/config"
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

	Name     string
	Theme    string
	Themes   []themeOption
	IsAdmin  bool
	Notation string

	Problem string
}

type themeOption struct {
	Value string
	Label string
	On    bool
}

// usersBody is the account table.
type usersBody struct {
	Tabs    []navLink
	Users   []userRow
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
	tabs := []navLink{{Label: "Perfil", URL: "/settings", On: current == "profile"}}
	if viewerOf(r).MayAdmin() {
		tabs = append(tabs, navLink{Label: "Usuarios", URL: "/settings/users", On: current == "users"})
	}
	if !s.cfg.ReadOnly {
		tabs = append(tabs, navLink{Label: "Importar y exportar", URL: "/import"})
	}
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
	body := settingsBody{
		Tabs:    s.settingsTabs(r, "profile"),
		Claim:   !v.In,
		Name:    v.User.Name,
		Theme:   v.User.Theme,
		IsAdmin: v.User.IsAdmin(),
		Problem: problem,
	}
	if !body.Claim {
		body.Themes = []themeOption{
			{account.ThemeSystem, "Como el sistema", v.User.Theme == account.ThemeSystem},
			{account.ThemeLight, "Claro", v.User.Theme == account.ThemeLight},
			{account.ThemeDark, "Oscuro", v.User.Theme == account.ThemeDark},
		}
	}
	if body.IsAdmin {
		if n, err := s.lib.Notation(r.Context()); err == nil {
			body.Notation = n
		}
		if body.Notation == "" {
			body.Notation = s.cfg.Notation
		}
	}
	s.render(w, r, status, "settings.html", s.page(r, "Ajustes", body))
}

// handleClaim creates the first administrator of an instance that has none.
//
// Open on purpose, and only while there is nobody: this is how an existing
// collection gets its first account. It refuses the instant one exists, so it
// cannot be used to add a second.
func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	if s.acc.Count() != 0 {
		s.fail(w, r, http.StatusForbidden, "Ya reclamada",
			"Esta instancia ya tiene cuentas. Pide a un administrador que te cree una.")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("user"))
	password := r.PostFormValue("password")
	if password != r.PostFormValue("repeat") {
		s.renderSettings(w, r, http.StatusBadRequest, "Las dos contraseñas no coinciden.")
		return
	}

	u, err := s.acc.Create(name, password, account.RoleAdmin)
	if err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(err))
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
			s.renderSettings(w, r, http.StatusBadRequest, accountProblem(err))
			return
		}
	}
	if err := s.acc.SetTheme(v.User.ID, r.PostFormValue("theme")); err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(err))
		return
	}
	s.redirect(w, r, "/settings", "profile")
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	if !v.In {
		s.refusePrivilege(w, r, accessWriter)
		return
	}
	next := r.PostFormValue("password")
	if next != r.PostFormValue("repeat") {
		s.renderSettings(w, r, http.StatusBadRequest, "Las dos contraseñas no coinciden.")
		return
	}
	err := func() error {
		_, e := s.hashSlot(func() (account.User, error) {
			return account.User{}, s.acc.ChangePassword(v.User.ID, r.PostFormValue("current"), next)
		})
		return e
	}()
	if err != nil {
		s.renderSettings(w, r, http.StatusBadRequest, accountProblem(err))
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

func (s *Server) handleSetNotation(w http.ResponseWriter, r *http.Request) {
	value := r.PostFormValue("notation")
	switch value {
	case config.NotationEnglish, config.NotationSolfege:
	default:
		s.renderSettings(w, r, http.StatusBadRequest, "Esa notación no existe.")
		return
	}
	if err := s.lib.SetNotation(r.Context(), value); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings", "notation")
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
	body := usersBody{Tabs: s.settingsTabs(r, "users"), Problem: problem}
	for _, u := range s.acc.List() {
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
		body.Users = append(body.Users, row)
	}
	s.render(w, r, status, "users.html", s.page(r, "Usuarios", body))
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
		s.renderUsers(w, r, http.StatusBadRequest, accountProblem(err))
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
		s.renderUsers(w, r, statusFor(err), accountProblem(err))
		return
	}
	s.redirect(w, r, "/settings/users", "role-changed")
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	_, err := s.hashSlot(func() (account.User, error) {
		return account.User{}, s.acc.SetPassword(r.PathValue("id"), r.PostFormValue("password"))
	})
	if err != nil {
		s.renderUsers(w, r, statusFor(err), accountProblem(err))
		return
	}
	s.redirect(w, r, "/settings/users", "user-reset")
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.acc.Delete(viewerOf(r).User.ID, r.PathValue("id")); err != nil {
		s.renderUsers(w, r, statusFor(err), accountProblem(err))
		return
	}
	s.redirect(w, r, "/settings/users", "user-deleted")
}

// accountProblem turns a domain error into something to show a person.
//
// The mapping is exhaustive over the errors the domain declares, and anything
// unrecognised becomes a generic sentence rather than leaking a Go error into the
// page.
func accountProblem(err error) string {
	switch {
	case errors.Is(err, account.ErrNameTaken):
		return "Ese nombre ya está en uso."
	case errors.Is(err, account.ErrNameInvalid):
		return "Un nombre puede tener entre 2 y 32 letras, números, puntos, guiones o guiones bajos."
	case errors.Is(err, account.ErrPasswordTooShort):
		return "La contraseña necesita al menos 10 caracteres."
	case errors.Is(err, account.ErrBadCredentials):
		return "La contraseña actual no es correcta."
	case errors.Is(err, account.ErrThemeInvalid):
		return "Ese tema no existe."
	case errors.Is(err, account.ErrLastAdmin):
		return "No puedes dejar la instancia sin administrador."
	case errors.Is(err, account.ErrSelfTarget):
		return "No puedes hacerte eso a tu propia cuenta. Pídeselo a otro administrador."
	case errors.Is(err, account.ErrNotFound):
		return "Esa cuenta ya no existe."
	}
	return "No se ha podido guardar el cambio."
}

func statusFor(err error) int {
	if errors.Is(err, account.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
