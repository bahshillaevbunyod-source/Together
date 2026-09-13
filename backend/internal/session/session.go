// Package session provides secure server-side sessions. The raw token is only
// ever sent to the client in a cookie; the database stores only its SHA-256
// hash. Nothing here logs raw tokens or hashes.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

// tokenBytes is the entropy of a raw session token (256 bits).
const tokenBytes = 32

// Session mirrors a row in the `sessions` table.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// GenerateToken returns a cryptographically secure, URL-safe raw token.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the hex SHA-256 hash of a raw token. Only the hash is
// persisted or compared server-side.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
