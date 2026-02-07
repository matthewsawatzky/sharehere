package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/util"
)

func (a *App) handleList(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	rel := a.parseRelative(r, "path")
	items, err := a.listDir(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"path":        rel,
		"entries":     items,
		"breadcrumbs": buildBreadcrumbs(rel),
	})
}

func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	rel := a.parseRelative(r, "path")
	abs, err := a.resolvePath(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if info.IsDir() {
		http.Redirect(w, r, fmt.Sprintf("%s?path=%s", a.route("/api/zip"), rel), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()))
	http.ServeFile(w, r, abs)
}

func (a *App) handlePreview(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	rel := a.parseRelative(r, "path")
	abs, err := a.resolvePath(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if info.IsDir() {
		a.writeJSON(w, http.StatusOK, map[string]any{"type": "directory"})
		return
	}
	file, err := os.Open(abs)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "open failed")
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ct := http.DetectContentType(head[:n])
	ext := strings.ToLower(filepath.Ext(abs))
	if strings.HasPrefix(ct, "image/") {
		a.writeJSON(w, http.StatusOK, map[string]any{"type": "image"})
		return
	}
	if isTextPreviewType(ct, ext) {
		const limit = 128 * 1024
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			a.writeError(w, http.StatusInternalServerError, "preview failed")
			return
		}
		buf := make([]byte, limit+1)
		n, _ := io.ReadFull(file, buf)
		truncated := n > limit
		if truncated {
			n = limit
		}
		a.writeJSON(w, http.StatusOK, map[string]any{
			"type":      "text",
			"content":   string(buf[:n]),
			"truncated": truncated,
		})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"type": "binary"})
}

func isTextPreviewType(contentType, ext string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	textExt := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".json": true, ".yml": true, ".yaml": true, ".toml": true, ".md": true,
		".txt": true, ".log": true, ".css": true, ".html": true, ".xml": true,
		".py": true, ".rb": true, ".rs": true, ".java": true, ".c": true,
		".cpp": true, ".h": true, ".sh": true, ".sql": true,
	}
	return textExt[ext]
}

func (a *App) handleZip(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodGet) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireBrowse(w, r, perms) {
		return
	}
	rel := a.parseRelative(r, "path")
	abs, err := a.resolvePath(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "not found")
		return
	}

	zipName := info.Name() + ".zip"
	if rel == "" {
		zipName = "sharehere-root.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipName))

	zw := zip.NewWriter(w)
	defer zw.Close()

	addFile := func(absPath, name string, info os.FileInfo) error {
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(name)
		hdr.Method = zip.Deflate
		writer, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(absPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(writer, f)
		return err
	}

	if !info.IsDir() {
		if err := addFile(abs, info.Name(), info); err != nil {
			a.writeError(w, http.StatusInternalServerError, "zip failed")
		}
		return
	}

	rootName := info.Name()
	if rel == "" {
		rootName = "root"
	}
	if err := filepath.WalkDir(abs, func(curr string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if curr == abs {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		relPath, err := filepath.Rel(abs, curr)
		if err != nil {
			return nil
		}
		zipPath := path.Join(rootName, filepath.ToSlash(relPath))
		return addFile(curr, zipPath, fi)
	}); err != nil {
		a.writeError(w, http.StatusInternalServerError, "zip failed")
	}
}

type renameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"newName"`
}

type singlePathRequest struct {
	Path string `json:"path"`
}

func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireWrite(w, perms, "upload") {
		return
	}
	user := a.currentUser(r)
	uploaded, issues, err := a.consumeMultipartUpload(w, r, settings, util.NormalizeRelPath(""))
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if user != nil {
		meta, _ := json.Marshal(map[string]any{"files": uploaded, "errors": issues})
		_ = a.store.RecordAudit(&user.ID, "upload", strings.Join(uploaded, ","), string(meta))
	} else {
		meta, _ := json.Marshal(map[string]any{"files": uploaded, "errors": issues})
		_ = a.store.RecordAudit(nil, "upload.guest", strings.Join(uploaded, ","), string(meta))
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"uploaded": uploaded, "errors": issues})
}

func (a *App) consumeMultipartUpload(w http.ResponseWriter, r *http.Request, settings db.AppSettings, forcedBaseRel string) ([]string, []string, error) {
	maxBytes := settings.MaxUploadSizeMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid multipart payload")
	}
	var allowRe, denyRe *regexp.Regexp
	if strings.TrimSpace(settings.UploadAllowRegex) != "" {
		allowRe, err = regexp.Compile(settings.UploadAllowRegex)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid allow regex")
		}
	}
	if strings.TrimSpace(settings.UploadDenyRegex) != "" {
		denyRe, err = regexp.Compile(settings.UploadDenyRegex)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid deny regex")
		}
	}

	baseRel := forcedBaseRel
	if baseRel == "" {
		baseRel = util.NormalizeRelPath(r.URL.Query().Get("path"))
	}
	if settings.UploadSubdir != "" {
		baseRel = path.Join(baseRel, util.NormalizeRelPath(settings.UploadSubdir))
	}
	baseRel = util.NormalizeRelPath(baseRel)

	uploaded := make([]string, 0)
	issues := make([]string, 0)

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return uploaded, issues, err
		}
		if part.FormName() == "path" && forcedBaseRel == "" {
			buf := &bytes.Buffer{}
			_, _ = io.CopyN(buf, part, 4096)
			v := util.NormalizeRelPath(buf.String())
			if settings.UploadSubdir != "" {
				v = path.Join(v, util.NormalizeRelPath(settings.UploadSubdir))
			}
			baseRel = util.NormalizeRelPath(v)
			part.Close()
			continue
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		filename := filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		if filename == "." || filename == "" {
			issues = append(issues, "invalid filename")
			part.Close()
			continue
		}
		if allowRe != nil && !allowRe.MatchString(filename) {
			issues = append(issues, fmt.Sprintf("rejected by allow policy: %s", filename))
			part.Close()
			continue
		}
		if denyRe != nil && denyRe.MatchString(filename) {
			issues = append(issues, fmt.Sprintf("rejected by deny policy: %s", filename))
			part.Close()
			continue
		}

		dirAbs, err := a.resolvePath(baseRel)
		if err != nil {
			issues = append(issues, fmt.Sprintf("invalid destination for %s", filename))
			part.Close()
			continue
		}
		if err := os.MkdirAll(dirAbs, 0o755); err != nil {
			issues = append(issues, fmt.Sprintf("mkdir failed for %s", filename))
			part.Close()
			continue
		}

		dest := filepath.Join(dirAbs, filename)
		if settings.CollisionPolicy != "overwrite" {
			dest = chooseCollisionPath(dest)
		}

		if err := writeUploadedFile(dest, part); err != nil {
			issues = append(issues, fmt.Sprintf("write failed for %s", filename))
			part.Close()
			continue
		}
		part.Close()

		relSaved, err := util.RelPathFromRoot(a.rootAbs, dest)
		if err != nil {
			relSaved = filename
		}
		uploaded = append(uploaded, relSaved)
		a.runVirusScanHook(settings.VirusScanCommand, dest)
	}
	return uploaded, issues, nil
}

func chooseCollisionPath(dest string) string {
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		return dest
	}
	ext := filepath.Ext(dest)
	base := strings.TrimSuffix(filepath.Base(dest), ext)
	dir := filepath.Dir(dest)
	for i := 1; i < 100000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext))
}

func writeUploadedFile(path string, src io.Reader) error {
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (a *App) runVirusScanHook(cmdline, filePath string) {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/C", cmdline)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", cmdline)
		}
		cmd.Env = append(os.Environ(), "SHAREHERE_FILE="+filePath)
		if err := cmd.Run(); err != nil {
			a.logger.Warn("virus scan hook failed", "file", filePath, "error", err)
		}
	}()
}

func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireWrite(w, perms, "delete") {
		return
	}
	var req singlePathRequest
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	rel := util.NormalizeRelPath(req.Path)
	if rel == "" {
		a.writeError(w, http.StatusBadRequest, "refusing to delete root")
		return
	}
	abs, err := a.resolvePath(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.RemoveAll(abs); err != nil {
		a.writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "file.delete", rel, "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleRename(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !a.requireWrite(w, perms, "rename") {
		return
	}
	var req renameRequest
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	rel := util.NormalizeRelPath(req.Path)
	newName := strings.TrimSpace(req.NewName)
	if rel == "" || newName == "" || newName != filepath.Base(newName) || strings.ContainsRune(newName, '/') || strings.ContainsRune(newName, '\\') {
		a.writeError(w, http.StatusBadRequest, "invalid rename request")
		return
	}
	oldAbs, err := a.resolvePath(rel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid source path")
		return
	}
	newRel := path.Join(path.Dir(rel), newName)
	newAbs, err := a.resolvePath(newRel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid target path")
		return
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		a.writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}
	if u := a.currentUser(r); u != nil {
		_ = a.store.RecordAudit(&u.ID, "file.rename", fmt.Sprintf("%s -> %s", rel, newRel), "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": newRel})
}

type shareCreateRequest struct {
	Path   string `json:"path"`
	Expiry string `json:"expiry"`
	Mode   string `json:"mode"`
}

func (a *App) handleCreateShareLink(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !perms.CanShare {
		a.writeError(w, http.StatusForbidden, "share link creation not allowed")
		return
	}
	var req shareCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	rel := util.NormalizeRelPath(req.Path)
	if _, err := a.resolvePath(rel); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	expiry := strings.TrimSpace(req.Expiry)
	if expiry == "" {
		expiry = settings.DefaultShareExpiry
	}
	d, err := time.ParseDuration(expiry)
	if err != nil || d <= 0 {
		a.writeError(w, http.StatusBadRequest, "invalid expiry duration")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "browse"
	}
	switch mode {
	case "browse", "download", "upload":
	default:
		a.writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	token, err := util.RandomToken(18)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	principal := a.currentPrincipal(r)
	var createdBy *int64
	if !principal.Anonymous {
		uid := principal.UserID
		createdBy = &uid
	}
	link := db.ShareLink{
		Token:     token,
		Path:      rel,
		Mode:      mode,
		CreatedBy: createdBy,
		ExpiresAt: time.Now().Add(d),
	}
	if err := a.store.CreateShareLink(link); err != nil {
		a.writeError(w, http.StatusInternalServerError, "failed to create link")
		return
	}
	if createdBy != nil {
		_ = a.store.RecordAudit(createdBy, "share.create", rel, mode)
	}
	url := a.absoluteURL(r, a.route("/s/"+token))
	a.writeJSON(w, http.StatusOK, map[string]any{"token": token, "url": url, "expiresAt": link.ExpiresAt})
}

func (a *App) handleRevokeShareLink(w http.ResponseWriter, r *http.Request) {
	if !a.enforceMethod(w, r, http.MethodPost) {
		return
	}
	if !a.verifyCSRF(w, r) {
		return
	}
	settings := a.effectiveSettings()
	perms := a.permissionsFor(r, settings)
	if !perms.CanShare {
		a.writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	link, err := a.store.GetShareLink(payload.Token)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "link not found")
		return
	}
	principal := a.currentPrincipal(r)
	if !perms.CanAdmin {
		if link.CreatedBy == nil || principal.UserID != *link.CreatedBy {
			a.writeError(w, http.StatusForbidden, "only owner or admin can revoke")
			return
		}
	}
	if err := a.store.RevokeShareLink(payload.Token); err != nil {
		a.writeError(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	if !principal.Anonymous {
		uid := principal.UserID
		_ = a.store.RecordAudit(&uid, "share.revoke", payload.Token, "")
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if a.opts.HTTPS || r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = net.JoinHostPort(a.opts.Bind, strconv.Itoa(a.opts.Port))
	}
	if a.opts.Host != "" {
		host = a.opts.Host
		if !strings.Contains(host, ":") {
			host = net.JoinHostPort(host, strconv.Itoa(a.opts.Port))
		}
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
