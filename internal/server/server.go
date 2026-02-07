package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/matthewsawatzky/sharehere/internal/auth"
	"github.com/matthewsawatzky/sharehere/internal/config"
	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/theme"
	"github.com/matthewsawatzky/sharehere/internal/util"
	"github.com/matthewsawatzky/sharehere/internal/webui"
)

const (
	sessionCookieName = "sharehere_session"
	anonTTL           = 24 * time.Hour
	authTTL           = 12 * time.Hour
	rememberTTL       = 30 * 24 * time.Hour
)

type ctxKey string

const (
	ctxSessionKey   ctxKey = "session"
	ctxUserKey      ctxKey = "user"
	ctxPrincipalKey ctxKey = "principal"
)

type App struct {
	opts      Options
	store     *db.Store
	logger    *slog.Logger
	templates *template.Template
	static    http.Handler
	rootAbs   string
}

func Run(ctx context.Context, opts Options) error {
	if opts.RootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get cwd: %w", err)
		}
		opts.RootDir = cwd
	}
	if opts.BasePath == "" {
		opts.BasePath = "/"
	}
	if !strings.HasPrefix(opts.BasePath, "/") {
		opts.BasePath = "/" + opts.BasePath
	}
	opts.BasePath = strings.TrimRight(opts.BasePath, "/")
	if opts.BasePath == "" {
		opts.BasePath = "/"
	}

	rootAbs, err := filepath.Abs(opts.RootDir)
	if err != nil {
		return fmt.Errorf("resolve root dir: %w", err)
	}
	store, err := db.Open(opts.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	handlerLevel := new(slog.LevelVar)
	handlerLevel.Set(parseLogLevel(opts.LogLevel))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: handlerLevel}))

	if opts.AuthMode == config.AuthOff {
		logger.Warn("authentication disabled; anyone on the network can access shared data")
	}

	if opts.GuestModeSet {
		if err := store.SetSetting("guest_mode", opts.GuestMode); err != nil {
			return err
		}
	}
	if opts.ReadOnlySet && opts.ReadOnly {
		if err := store.SetSetting("read_only", "true"); err != nil {
			return err
		}
	}

	admins, err := store.AdminCount()
	if err == nil && admins == 0 && opts.AuthMode != config.AuthOff {
		logger.Warn("no active admin account found; run `sharehere init` or `sharehere user add --role admin`")
	}

	tmpl, err := template.ParseFS(webui.FS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	staticFS, err := fs.Sub(webui.FS, "static")
	if err != nil {
		return fmt.Errorf("open static fs: %w", err)
	}

	app := &App{
		opts:      opts,
		store:     store,
		logger:    logger,
		templates: tmpl,
		static:    http.FileServer(http.FS(staticFS)),
		rootAbs:   rootAbs,
	}

	mux := http.NewServeMux()
	mux.Handle(app.route("/static/"), http.StripPrefix(app.route("/static/"), app.static))
	mux.HandleFunc(app.route("/"), app.handleIndex)
	mux.HandleFunc(app.route("/login"), app.handleLogin)
	mux.HandleFunc(app.route("/logout"), app.handleLogout)
	mux.HandleFunc(app.route("/admin"), app.handleAdminPage)

	mux.HandleFunc(app.route("/api/me"), app.handleMe)
	mux.HandleFunc(app.route("/api/themes"), app.handleThemes)
	mux.HandleFunc(app.route("/api/list"), app.handleList)
	mux.HandleFunc(app.route("/api/download"), app.handleDownload)
	mux.HandleFunc(app.route("/api/preview"), app.handlePreview)
	mux.HandleFunc(app.route("/api/zip"), app.handleZip)
	mux.HandleFunc(app.route("/api/upload"), app.handleUpload)
	mux.HandleFunc(app.route("/api/delete"), app.handleDelete)
	mux.HandleFunc(app.route("/api/rename"), app.handleRename)
	mux.HandleFunc(app.route("/api/share/create"), app.handleCreateShareLink)
	mux.HandleFunc(app.route("/api/share/revoke"), app.handleRevokeShareLink)

	mux.HandleFunc(app.route("/api/admin/settings"), app.handleAdminSettings)
	mux.HandleFunc(app.route("/api/admin/users"), app.handleAdminUsers)
	mux.HandleFunc(app.route("/api/admin/users/create"), app.handleAdminCreateUser)
	mux.HandleFunc(app.route("/api/admin/users/password"), app.handleAdminSetPassword)
	mux.HandleFunc(app.route("/api/admin/users/disable"), app.handleAdminDisableUser)
	mux.HandleFunc(app.route("/api/admin/users/delete"), app.handleAdminDeleteUser)
	mux.HandleFunc(app.route("/api/admin/links"), app.handleAdminLinks)
	mux.HandleFunc(app.route("/api/admin/audit"), app.handleAdminAudit)

	mux.HandleFunc(app.route("/s/"), app.handleShare)

	handler := app.recoverer(app.securityHeaders(app.sessionMiddleware(mux)))
	addr := net.JoinHostPort(opts.Bind, strconv.Itoa(opts.Port))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       90 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if opts.HTTPS {
			errCh <- httpServer.ListenAndServeTLS(opts.CertFile, opts.KeyFile)
			return
		}
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (a *App) route(p string) string {
	if a.opts.BasePath == "/" {
		return p
	}
	if p == "/" {
		return a.opts.BasePath + "/"
	}
	return a.opts.BasePath + p
}

func (a *App) parseRelative(r *http.Request, key string) string {
	return util.NormalizeRelPath(r.URL.Query().Get(key))
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.logger.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie(sessionCookieName)
		token := ""
		if cookie != nil {
			token = cookie.Value
		}
		session, err := a.store.GetSession(token)
		if err != nil {
			session, err = a.newAnonymousSession(r)
			if err != nil {
				a.writeError(w, http.StatusInternalServerError, "session failure")
				return
			}
			if err := a.store.CreateSession(session); err != nil {
				a.writeError(w, http.StatusInternalServerError, "session failure")
				return
			}
			a.setSessionCookie(w, session)
		} else {
			expires := time.Now().Add(anonTTL)
			if session.UserID != nil {
				if session.Remember {
					expires = time.Now().Add(rememberTTL)
				} else {
					expires = time.Now().Add(authTTL)
				}
			}
			session.ExpiresAt = expires
			_ = a.store.TouchSession(session.Token, session.ExpiresAt)
			a.setSessionCookie(w, session)
		}

		principal := auth.Principal{Anonymous: true, Role: "guest", Username: "guest"}
		var u *db.User
		if session.UserID != nil {
			user, err := a.store.GetUserByID(*session.UserID)
			if err == nil && !user.Disabled {
				u = &user
				principal = auth.Principal{UserID: user.ID, Username: user.Username, Role: user.Role}
			}
		}
		if a.opts.AuthMode == config.AuthOff {
			principal = auth.Principal{UserID: 0, Username: "unsafe-admin", Role: auth.RoleAdmin, Anonymous: false}
		}
		ctx := context.WithValue(r.Context(), ctxSessionKey, session)
		ctx = context.WithValue(ctx, ctxPrincipalKey, principal)
		if u != nil {
			ctx = context.WithValue(ctx, ctxUserKey, *u)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) newAnonymousSession(r *http.Request) (db.Session, error) {
	token, err := util.RandomToken(32)
	if err != nil {
		return db.Session{}, err
	}
	csrf, err := util.RandomToken(24)
	if err != nil {
		return db.Session{}, err
	}
	return db.Session{
		Token:     token,
		CSRFToken: csrf,
		IP:        remoteIP(r),
		UserAgent: r.UserAgent(),
		ExpiresAt: time.Now().Add(anonTTL),
	}, nil
}

func remoteIP(r *http.Request) string {
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (a *App) setSessionCookie(w http.ResponseWriter, session db.Session) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     a.opts.BasePath,
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   a.opts.HTTPS,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     a.opts.BasePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.opts.HTTPS,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) currentSession(r *http.Request) db.Session {
	v := r.Context().Value(ctxSessionKey)
	if v == nil {
		return db.Session{}
	}
	s, _ := v.(db.Session)
	return s
}

func (a *App) currentPrincipal(r *http.Request) auth.Principal {
	v := r.Context().Value(ctxPrincipalKey)
	if v == nil {
		return auth.Principal{Anonymous: true, Role: "guest", Username: "guest"}
	}
	p, _ := v.(auth.Principal)
	return p
}

func (a *App) currentUser(r *http.Request) *db.User {
	v := r.Context().Value(ctxUserKey)
	if v == nil {
		return nil
	}
	u, ok := v.(db.User)
	if !ok {
		return nil
	}
	return &u
}

func (a *App) effectiveSettings() db.AppSettings {
	s, err := a.store.GetAppSettings()
	if err != nil {
		return db.AppSettings{
			GuestMode:          config.GuestOff,
			MaxUploadSizeMB:    1024,
			CollisionPolicy:    config.CollisionRename,
			DefaultShareExpiry: "24h",
			Theme:              "light",
		}
	}
	if a.opts.ReadOnly {
		s.ReadOnly = true
	}
	return s
}

func (a *App) themeFromSettings(settings db.AppSettings) theme.Theme {
	overrides := theme.Overrides{}
	if strings.TrimSpace(settings.ThemeOverridesJSON) != "" {
		_ = json.Unmarshal([]byte(settings.ThemeOverridesJSON), &overrides)
	}
	t, err := theme.Resolve(settings.Theme, overrides)
	if err != nil {
		t, _ = theme.Resolve("light", theme.Overrides{})
	}
	return t
}

func (a *App) permissionsFor(r *http.Request, settings db.AppSettings) Permissions {
	principal := a.currentPrincipal(r)
	if a.opts.AuthMode == config.AuthOff {
		p := Permissions{CanBrowse: true, CanUpload: true, CanDelete: true, CanRename: true, CanShare: true, CanAdmin: true, ReadOnly: settings.ReadOnly}
		if p.ReadOnly {
			p.CanUpload, p.CanDelete, p.CanRename = false, false, false
		}
		return p
	}

	perms := Permissions{ReadOnly: settings.ReadOnly}
	if !principal.Anonymous {
		perms.CanBrowse = true
		perms.CanShare = true
		perms.CanUpload = !settings.ReadOnly
		perms.CanDelete = settings.AllowDelete && !settings.ReadOnly
		perms.CanRename = settings.AllowRename && !settings.ReadOnly
		perms.CanAdmin = principal.Role == auth.RoleAdmin
		if perms.CanAdmin && !settings.ReadOnly {
			perms.CanDelete = true
			perms.CanRename = true
		}
		if perms.CanAdmin && settings.ReadOnly {
			perms.CanDelete = false
			perms.CanRename = false
			perms.CanUpload = false
		}
		return perms
	}

	switch settings.GuestMode {
	case config.GuestRead:
		perms.CanBrowse = true
	case config.GuestUpload:
		perms.CanBrowse = true
		perms.CanUpload = !settings.ReadOnly
	}
	return perms
}

func (a *App) requireBrowse(w http.ResponseWriter, r *http.Request, perms Permissions) bool {
	if perms.CanBrowse {
		return true
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") || strings.HasPrefix(r.URL.Path, a.route("/api/")) {
		a.writeError(w, http.StatusUnauthorized, "authentication required")
	} else {
		http.Redirect(w, r, a.route("/login"), http.StatusSeeOther)
	}
	return false
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request, perms Permissions) bool {
	if perms.CanAdmin {
		return true
	}
	a.writeError(w, http.StatusForbidden, "admin access required")
	return false
}

func (a *App) requireWrite(w http.ResponseWriter, perms Permissions, action string) bool {
	if perms.ReadOnly {
		a.writeError(w, http.StatusForbidden, "read-only mode enabled")
		return false
	}
	switch action {
	case "upload":
		if !perms.CanUpload {
			a.writeError(w, http.StatusForbidden, "upload not allowed")
			return false
		}
	case "delete":
		if !perms.CanDelete {
			a.writeError(w, http.StatusForbidden, "delete not allowed")
			return false
		}
	case "rename":
		if !perms.CanRename {
			a.writeError(w, http.StatusForbidden, "rename not allowed")
			return false
		}
	}
	return true
}

func (a *App) enforceMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}

func (a *App) verifyCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if strings.HasPrefix(r.URL.Path, a.route("/s/")) {
		// Share token endpoints are bearer-token based.
		return true
	}
	session := a.currentSession(r)
	provided := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if provided == "" {
		_ = r.ParseForm()
		provided = strings.TrimSpace(r.FormValue("_csrf"))
	}
	if provided == "" || session.CSRFToken == "" || provided != session.CSRFToken {
		a.writeError(w, http.StatusForbidden, "csrf validation failed")
		return false
	}
	return true
}

func (a *App) resolvePath(rel string) (string, error) {
	return util.SafeJoin(a.rootAbs, rel)
}

func (a *App) listDir(rel string) ([]fileEntry, error) {
	abs, err := a.resolvePath(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	items := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		itemRel := path.Join(rel, e.Name())
		itemRel = strings.TrimPrefix(itemRel, "/")
		items = append(items, fileEntry{
			Name:    e.Name(),
			RelPath: util.NormalizeRelPath(itemRel),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Ext:     strings.ToLower(filepath.Ext(e.Name())),
		})
	}
	return items, nil
}

func buildBreadcrumbs(rel string) []breadcrumb {
	rel = util.NormalizeRelPath(rel)
	crumbs := []breadcrumb{{Name: "/", Path: ""}}
	if rel == "" {
		return crumbs
	}
	parts := strings.Split(rel, "/")
	cur := ""
	for _, p := range parts {
		cur = path.Join(cur, p)
		crumbs = append(crumbs, breadcrumb{Name: p, Path: cur})
	}
	return crumbs
}

func (a *App) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *App) writeError(w http.ResponseWriter, status int, message string) {
	a.writeJSON(w, status, map[string]any{"error": message})
}
