package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	saltRaw, err := randomBytes(saltLen)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), saltRaw, argonTime, argonMemory, argonThreads, argonKeyLen)
	salt := base64.RawStdEncoding.EncodeToString(saltRaw)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, salt, hashB64)
	return encoded, nil
}

func VerifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}
	if parts[1] != "argon2id" {
		return false, fmt.Errorf("unsupported hash algorithm")
	}

	var memory uint32
	var timeCost uint32
	var threads uint8
	for _, pair := range strings.Split(parts[3], ",") {
		kvs := strings.SplitN(pair, "=", 2)
		if len(kvs) != 2 {
			continue
		}
		switch kvs[0] {
		case "m":
			v, err := strconv.ParseUint(kvs[1], 10, 32)
			if err != nil {
				return false, err
			}
			memory = uint32(v)
		case "t":
			v, err := strconv.ParseUint(kvs[1], 10, 32)
			if err != nil {
				return false, err
			}
			timeCost = uint32(v)
		case "p":
			v, err := strconv.ParseUint(kvs[1], 10, 8)
			if err != nil {
				return false, err
			}
			threads = uint8(v)
		}
	}
	if memory == 0 || timeCost == 0 || threads == 0 {
		return false, fmt.Errorf("invalid argon2 parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(hash)))
	ok := subtle.ConstantTimeCompare(candidate, hash) == 1
	return ok, nil
}
