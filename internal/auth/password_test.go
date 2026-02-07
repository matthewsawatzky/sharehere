package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !ok {
		t.Fatalf("expected password verification to pass")
	}
	ok, err = VerifyPassword(hash, "bad pass")
	if err != nil {
		t.Fatalf("verify bad password returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected wrong password to fail")
	}
}
