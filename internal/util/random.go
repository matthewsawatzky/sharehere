package util

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// RandomToken returns a URL-safe random token with n random bytes of entropy.
func RandomToken(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("token size must be > 0")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
