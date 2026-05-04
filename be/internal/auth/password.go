package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024 // 64 MiB
	argonTime    = 3
	argonThreads = 2
	argonSaltLen = 16
	argonKeyLen  = 32
)

var (
	ErrMalformedHash = errors.New("auth: malformed PHC hash")
	ErrHashMismatch  = errors.New("auth: password does not match")
)

// Hash returns an Argon2id PHC-format hash for the given plaintext password.
func Hash(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	enc := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt),
		enc.EncodeToString(key),
	), nil
}

// Verify returns nil if plain matches the stored PHC hash, ErrHashMismatch on mismatch,
// or ErrMalformedHash if the hash string cannot be parsed.
func Verify(hash, plain string) error {
	parts := strings.Split(hash, "$")
	// Expected: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt-b64>", "<key-b64>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrMalformedHash
	}

	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return ErrMalformedHash
	}

	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return ErrMalformedHash
	}
	expectedKey, err := enc.DecodeString(parts[5])
	if err != nil {
		return ErrMalformedHash
	}

	actualKey := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(expectedKey)))

	if subtle.ConstantTimeCompare(actualKey, expectedKey) != 1 {
		return ErrHashMismatch
	}
	return nil
}
