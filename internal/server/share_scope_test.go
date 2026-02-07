package server

import "testing"

func TestResolveScopedSharePath(t *testing.T) {
	v, err := resolveScopedSharePath("docs", "images")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "docs/images" {
		t.Fatalf("unexpected scoped path: %s", v)
	}
	if _, err := resolveScopedSharePath("docs", "../../etc"); err == nil {
		t.Fatalf("expected scope escape to fail")
	}
}
