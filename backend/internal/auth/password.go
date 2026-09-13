// Package auth provides password hashing and verification. It never logs
// passwords or hashes.
package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is the minimum number of characters.
	MinPasswordLength = 8
	// MaxPasswordBytes is bcrypt's hard input limit.
	MaxPasswordBytes = 72
)

var (
	// ErrPasswordTooShort is returned when a password has fewer than
	// MinPasswordLength characters.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	// ErrPasswordTooLong is returned when a password exceeds MaxPasswordBytes.
	ErrPasswordTooLong = errors.New("password must be at most 72 bytes")
)

// HashPassword validates the password policy and returns a bcrypt hash.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the bcrypt hash. It returns
// nil on a match and a non-nil error otherwise.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
