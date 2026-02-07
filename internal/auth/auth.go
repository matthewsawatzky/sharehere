package auth

import (
	"crypto/rand"
	"fmt"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type Principal struct {
	UserID    int64
	Username  string
	Role      string
	Anonymous bool
}

func (p Principal) IsAdmin() bool {
	return !p.Anonymous && p.Role == RoleAdmin
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("random bytes: %w", err)
	}
	return b, nil
}
