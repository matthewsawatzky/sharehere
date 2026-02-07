package server

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/util"
)

func (a *App) handleShare(w http.ResponseWriter, r *http.Request) {
	prefix := a.route("/s/")
	raw := strings.TrimPrefix(r.URL.Path, prefix)
	raw = strings.TrimPrefix(raw, "/")
	parts := strings.Split(raw, "/")
	if len(parts) == 0 || parts[0] == "" {
		a.writeError(w, http.StatusNotFound, "link not found")
		return
	}
	token := parts[0]
	suffix := ""
	if len(parts) > 1 {
		suffix = parts[1]
	}

	link, err := a.store.GetShareLink(token)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if link.Revoked || time.Now().After(link.ExpiresAt) {
		a.writeError(w, http.StatusGone, "share link expired or revoked")
		return
	}
	_ = a.store.MarkShareLinkAccessed(token)

	if suffix == "upload" {
		if link.Mode != "upload" {
			a.writeError(w, http.StatusForbidden, "upload not allowed for this link")
			return
		}
		a.handleShareUpload(w, r, link)
		return
	}

	switch link.Mode {
	case "upload":
		if r.Method != http.MethodGet {
			a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := a.templates.ExecuteTemplate(w, "share_upload.html", map[string]any{
			"BasePath": a.opts.BasePath,
			"Token":    link.Token,
			"Path":     link.Path,
		}); err != nil {
			a.writeError(w, http.StatusInternalServerError, "render failed")
		}
	case "download":
		if r.Method != http.MethodGet {
			a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.serveRelAsDownload(w, r, link.Path)
	case "browse":
		a.handleShareBrowse(w, r, link)
	default:
		a.writeError(w, http.StatusNotFound, "unknown link mode")
	}
}

func (a *App) handleShareUpload(w http.ResponseWriter, r *http.Request, link db.ShareLink) {
	if r.Method != http.MethodPost {
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings := a.effectiveSettings()
	if settings.ReadOnly {
		a.writeError(w, http.StatusForbidden, "read-only mode enabled")
		return
	}
	base := link.Path
	if abs, err := a.resolvePath(base); err == nil {
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
			base = path.Dir(base)
		}
	}
	uploaded, issues, err := a.consumeMultipartUpload(w, r, settings, util.NormalizeRelPath(base))
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	meta := fmt.Sprintf("token=%s", link.Token)
	_ = a.store.RecordAudit(link.CreatedBy, "share.upload", strings.Join(uploaded, ","), meta)
	a.writeJSON(w, http.StatusOK, map[string]any{"uploaded": uploaded, "errors": issues})
}

func (a *App) handleShareBrowse(w http.ResponseWriter, r *http.Request, link db.ShareLink) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sub := util.NormalizeRelPath(r.URL.Query().Get("p"))
	scopedRel, err := resolveScopedSharePath(link.Path, sub)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if r.URL.Query().Get("download") != "" {
		a.serveRelAsDownload(w, r, scopedRel)
		return
	}
	abs, err := a.resolvePath(scopedRel)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if !info.IsDir() {
		a.serveRelAsDownload(w, r, scopedRel)
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "cannot read directory")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>sharehere link</title><link rel=\"stylesheet\" href=\"%s/static/style.css\"></head><body><main class=\"panel\" style=\"margin:1rem;max-width:960px\"><h1>Shared folder</h1>", html.EscapeString(a.opts.BasePath))
	fmt.Fprintf(w, "<p><strong>Path:</strong> <code>%s</code></p>", html.EscapeString(scopedRel))
	zipURL := fmt.Sprintf("%s/s/%s?p=%s&download=1", a.opts.BasePath, link.Token, url.QueryEscape(scopedRel))
	fmt.Fprintf(w, "<p><a class=\"button\" href=\"%s\">Download current path</a></p>", html.EscapeString(zipURL))
	fmt.Fprint(w, "<ul>")
	if scopedRel != link.Path {
		parent := path.Dir(scopedRel)
		if parent == "." {
			parent = ""
		}
		up := fmt.Sprintf("%s/s/%s?p=%s", a.opts.BasePath, link.Token, url.QueryEscape(parent))
		fmt.Fprintf(w, "<li><a href=\"%s\">..</a></li>", html.EscapeString(up))
	}
	for _, e := range entries {
		name := e.Name()
		next := path.Join(scopedRel, name)
		next = util.NormalizeRelPath(next)
		href := fmt.Sprintf("%s/s/%s?p=%s", a.opts.BasePath, link.Token, url.QueryEscape(next))
		if !e.IsDir() {
			href = fmt.Sprintf("%s/s/%s?p=%s&download=1", a.opts.BasePath, link.Token, url.QueryEscape(next))
		}
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(w, "<li><a href=\"%s\">%s%s</a></li>", html.EscapeString(href), html.EscapeString(name), suffix)
	}
	fmt.Fprint(w, "</ul></main></body></html>")
}

func resolveScopedSharePath(base, sub string) (string, error) {
	base = util.NormalizeRelPath(base)
	sub = util.NormalizeRelPath(sub)
	if sub == "" {
		return base, nil
	}
	joined := util.NormalizeRelPath(path.Join(base, sub))
	if base == "" {
		return joined, nil
	}
	if joined == base || strings.HasPrefix(joined, base+"/") {
		return joined, nil
	}
	return "", fmt.Errorf("path escapes scope")
}

func (a *App) serveRelAsDownload(w http.ResponseWriter, r *http.Request, rel string) {
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
	if !info.IsDir() {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()))
		http.ServeFile(w, r, abs)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name()+".zip"))
	zw := zip.NewWriter(w)
	defer zw.Close()
	_ = filepath.WalkDir(abs, func(curr string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 {
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
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return nil
		}
		hdr.Name = filepath.ToSlash(path.Join(info.Name(), relPath))
		hdr.Method = zip.Deflate
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil
		}
		f, err := os.Open(curr)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, _ = io.Copy(fw, f)
		return nil
	})
}
