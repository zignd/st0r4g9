// Package auth handles API-key generation, hashing, and the Fiber middleware
// that authenticates requests and scopes them to the owning key.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// KeyPrefix is prepended to every generated key so it is recognizable.
const KeyPrefix = "st0r_"

// GenerateKey returns a new random API key of the form "st0r_<64 hex chars>".
// The returned string is the secret shown to the user exactly once.
func GenerateKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return KeyPrefix + hex.EncodeToString(buf), nil
}

// Hash returns the hex-encoded SHA-256 of a raw key, as stored in the database.
func Hash(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// DisplayPrefix returns a short, non-secret prefix of a raw key for display
// (e.g. "st0r_1a2b3c4d"), safe to persist and echo back.
func DisplayPrefix(rawKey string) string {
	const shown = len(KeyPrefix) + 8
	if len(rawKey) < shown {
		return rawKey
	}
	return rawKey[:shown]
}
