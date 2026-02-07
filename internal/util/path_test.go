package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinBlocksTraversal(t *testing.T) {
	root := t.TempDir()
	joined, err := SafeJoin(root, "../../etc/passwd")
	if err != nil {
		t.Fatalf("expected normalized path under root, got error: %v", err)
	}
	if filepath.Dir(joined) == "/etc" {
		t.Fatalf("path escaped root: %s", joined)
	}
}

func TestSafeJoinSymlinkEscape(t *testing.T) {
	if os.Getenv("CI") == "windows" {
		t.Skip("symlink creation requires privileges on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if _, err := SafeJoin(root, "link/secret.txt"); err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}
}
