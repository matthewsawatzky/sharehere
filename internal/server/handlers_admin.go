package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/matthewsawatzky/sharehere/internal/auth"
	"github.com/matthewsawatzky/sharehere/internal/config"
	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/theme"
)

func (a *App) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
		return
	case http.MethodPost:
		if !a.verifyCSRF(w, r) {
			return
		}
	default:
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var next db.AppSettings
	if err := decodeJSONBody(r, &next); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if next.MaxUploadSizeMB <= 0 {
		a.writeError(w, http.StatusBadRequest, "max_upload_size_mb must be positive")
		return
	}
	switch next.GuestMode {
	case config.GuestOff, config.GuestRead, config.GuestUpload:
	default:
		a.writeError(w, http.StatusBadRequest, "invalid guest mode")
		return
	}
	switch next.CollisionPolicy {
	case config.CollisionRename, config.CollisionOverwrite:
	default:
		a.writeError(w, http.StatusBadRequest, "invalid collision policy")
		return
	}
	if strings.TrimSpace(next.DefaultShareExpiry) == "" {
		next.DefaultShareExpiry = "24h"
	}
	if _, err := time.ParseDuration(next.DefaultShareExpiry); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid default share expiry")
		return
	}
	if strings.TrimSpace(next.UploadAllowRegex) != "" {
		if _, err := regexp.Compile(next.UploadAllowRegex); err != nil {
			a.writeError(w, http.StatusBadRequest, "invalid upload_allow_regex")
			return
		}
	}
	if strings.TrimSpace(next.UploadDenyRegex) != "" {
		if _, err := regexp.Compile(next.UploadDenyRegex); err != nil {
			a.writeError(w, http.StatusBadRequest, "invalid upload_deny_regex")
			return
		}
	}
	if strings.TrimSpace(next.Theme) == "" {
		next.Theme = "light"
	}
	overrides := theme.Overrides{}
	if strings.TrimSpace(next.ThemeOverridesJSON) == "" {
		next.ThemeOverridesJSON = "{}"
	}
	if err := json.Unmarshal([]byte(next.ThemeOverridesJSON), &overrides); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid theme_overrides_json")
		return
	}
	if _, err := theme.Resolve(next.Theme, overrides); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid theme")
		return
	}
	if err := a.store.SetAppSettings(next); err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "admin.settings.update", "settings", "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	users, err := a.store.ListUsers()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if req.Username == "" || req.Password == "" {
		a.writeError(w, http.StatusBadRequest, "username/password required")
		return
	}
	switch req.Role {
	case auth.RoleAdmin, auth.RoleUser:
	default:
		req.Role = auth.RoleUser
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := a.store.CreateUser(req.Username, hash, req.Role)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "failed to create user")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "admin.user.create", req.Username, req.Role)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (a *App) handleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.SetUserPassword(req.Username, hash); err != nil {
		if err == sql.ErrNoRows {
			a.writeError(w, http.StatusNotFound, "user not found")
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "admin.user.password", req.Username, "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAdminDisableUser(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	target, err := a.store.GetUserByUsername(req.Username)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.Role == auth.RoleAdmin && req.Disabled && !target.Disabled {
		admins, err := a.store.AdminCount()
		if err == nil && admins <= 1 {
			a.writeError(w, http.StatusBadRequest, "cannot disable last active admin")
			return
		}
	}
	if err := a.store.SetUserDisabled(req.Username, req.Disabled); err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "admin.user.disable", req.Username, strconv.FormatBool(req.Disabled))
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	target, err := a.store.GetUserByUsername(req.Username)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.Role == auth.RoleAdmin {
		admins, err := a.store.AdminCount()
		if err == nil && admins <= 1 {
			a.writeError(w, http.StatusBadRequest, "cannot remove last active admin")
			return
		}
	}
	if err := a.store.DeleteUser(req.Username); err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to remove user")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "admin.user.delete", req.Username, "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAdminLinks(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	links, err := a.store.ListShareLinks()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to list links")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (a *App) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireAdmin(w, r, perms) {
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	logs, err := a.store.ListAudit(limit)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
