package util

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// NormalizeRelPath normalizes user input into a slash-separated, rooted-relative path.
func NormalizeRelPath(p string) string {
	clean := strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "/" {
		return ""
	}
	clean = strings.TrimPrefix(path.Clean("/"+clean), "/")
	if clean == "." {
		return ""
	}
	return clean
}

func absPath(p string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(a), nil
}

func symlinkAwarePath(p string) string {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return real
}

func withinRoot(root, target string) bool {
	if root == target {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(target, root+sep)
}

// SafeJoin joins rel under root and prevents traversal outside root, including via symlinks.
func SafeJoin(root, rel string) (string, error) {
	if strings.ContainsRune(rel, '\x00') {
		return "", errors.New("invalid path")
	}
	normalized := NormalizeRelPath(rel)
	rootAbs, err := absPath(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	joined := filepath.Join(rootAbs, filepath.FromSlash(normalized))
	joinedAbs, err := absPath(joined)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !withinRoot(rootAbs, joinedAbs) {
		return "", errors.New("path escapes root")
	}

	rootReal := symlinkAwarePath(rootAbs)
	targetReal := joinedAbs
	if _, err := os.Stat(joinedAbs); err == nil {
		targetReal = symlinkAwarePath(joinedAbs)
	} else {
		// For non-existing paths (uploads/new files), validate parent directory.
		parent := symlinkAwarePath(filepath.Dir(joinedAbs))
		if !withinRoot(rootReal, parent) {
			return "", errors.New("path escapes root via symlink")
		}
	}
	if !withinRoot(rootReal, targetReal) {
		return "", errors.New("path escapes root via symlink")
	}
	return joinedAbs, nil
}

func RelPathFromRoot(root, absolute string) (string, error) {
	rootAbs, err := absPath(root)
	if err != nil {
		return "", err
	}
	abs, err := absPath(absolute)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", nil
	}
	return rel, nil
}
