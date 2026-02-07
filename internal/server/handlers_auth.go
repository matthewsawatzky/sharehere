package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/matthewsawatzky/sharehere/internal/auth"
	"github.com/matthewsawatzky/sharehere/internal/config"
	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/theme"
	"github.com/matthewsawatzky/sharehere/internal/util"
)

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	data := map[string]any{
		"BasePath": a.opts.BasePath,
		"Version":  a.opts.Version,
	}
	if err := a.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		a.writeError(w, http.StatusInternalServerError, "render failed")
	}
}

func (a *App) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	data := map[string]any{"BasePath": a.opts.BasePath, "Version": a.opts.Version}
	if err := a.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
		a.writeError(w, http.StatusInternalServerError, "render failed")
	}
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	session := a.currentSession(r)
	if a.opts.AuthMode == config.AuthOff {
		http.Redirect(w, r, a.route("/"), http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		if !a.currentPrincipal(r).Anonymous {
			http.Redirect(w, r, a.route("/"), http.StatusSeeOther)
			return
		}
		_ = a.templates.ExecuteTemplate(w, "login.html", map[string]any{
			"BasePath":  a.opts.BasePath,
			"CSRFToken": session.CSRFToken,
		})
		return
	}
	if r.Method != http.MethodPost {
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid form")
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")
	remember := r.FormValue("remember") == "1" || r.FormValue("remember") == "on"
	if username == "" || password == "" {
		a.renderLoginError(w, session.CSRFToken, "username and password are required")
		return
	}

	key := fmt.Sprintf("%s|%s", remoteIP(r), username)
	locked, retryAfter, err := a.store.CheckLoginAllowed(key)
	if err == nil && locked {
		a.renderLoginError(w, session.CSRFToken, fmt.Sprintf("too many attempts, retry in %s", retryAfter.Round(time.Second)))
		return
	}

	user, err := a.store.GetUserByUsername(username)
	if err != nil || user.Disabled {
		a.failLogin(w, session.CSRFToken, key, username)
		return
	}
	ok, err := auth.VerifyPassword(user.PasswordHash, password)
	if err != nil || !ok {
		a.failLogin(w, session.CSRFToken, key, username)
		return
	}
	_ = a.store.ResetLoginAttempts(key)

	token, err := util.RandomToken(32)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "session failure")
		return
	}
	csrf, err := util.RandomToken(24)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "session failure")
		return
	}
	expires := time.Now().Add(authTTL)
	if remember {
		expires = time.Now().Add(rememberTTL)
	}
	uid := user.ID
	newSess := db.Session{
		Token:     token,
		UserID:    &uid,
		CSRFToken: csrf,
		Remember:  remember,
		IP:        remoteIP(r),
		UserAgent: r.UserAgent(),
		ExpiresAt: expires,
	}
	if err := a.store.RotateSession(session.Token, newSess); err != nil {
		a.writeError(w, http.StatusInternalServerError, "session failure")
		return
	}
	a.setSessionCookie(w, newSess)
	_ = a.store.RecordAudit(&uid, "login.success", username, "")
	http.Redirect(w, r, a.route("/"), http.StatusSeeOther)
}

func (a *App) failLogin(w http.ResponseWriter, csrfToken, key, username string) {
	lock, _ := a.store.RegisterFailedLogin(key)
	_ = a.store.RecordAudit(nil, "login.failed", username, "")
	msg := "invalid credentials"
	if lock > 0 {
		msg = fmt.Sprintf("invalid credentials. account locked for %s", lock.Round(time.Second))
	}
	a.renderLoginError(w, csrfToken, msg)
}

func (a *App) renderLoginError(w http.ResponseWriter, csrfToken, message string) {
	w.WriteHeader(http.StatusUnauthorized)
	_ = a.templates.ExecuteTemplate(w, "login.html", map[string]any{
		"BasePath":  a.opts.BasePath,
		"CSRFToken": csrfToken,
		"Error":     message,
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	session := a.currentSession(r)
	user := a.currentUser(r)
	if user != nil {
		_ = a.store.RecordAudit(&user.ID, "logout", user.Username, "")
	}
	_ = a.store.DeleteSession(session.Token)
	a.clearSessionCookie(w)
	http.Redirect(w, r, a.route("/login"), http.StatusSeeOther)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	session := a.currentSession(r)
	principal := a.currentPrincipal(r)
	th := a.themeFromSettings(settings)

	role := "guest"
	if !principal.Anonymous {
		role = principal.Role
	}
	payload := map[string]any{
		"authenticated": !principal.Anonymous,
		"username":      principal.Username,
		"role":          role,
		"csrfToken":     session.CSRFToken,
		"guestMode":     settings.GuestMode,
		"permissions":   perms,
		"theme":         map[string]any{"name": th.Name, "label": th.Label, "css_variables": th.CSSVariables},
		"rootPath":      a.rootAbs,
	}
	a.writeJSON(w, http.StatusOK, payload)
}

func (a *App) handleThemes(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"themes": theme.List()})
}
