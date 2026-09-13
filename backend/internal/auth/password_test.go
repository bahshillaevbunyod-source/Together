package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordSuccess(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}
	if hash == "correct horse" {
		t.Fatal("hash must not equal the plaintext password")
	}
}

func TestCheckPasswordCorrect(t *testing.T) {
	const pw = "correct horse"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := CheckPassword(hash, pw); err != nil {
		t.Fatalf("expected password to match, got %v", err)
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := CheckPassword(hash, "wrong horse"); err == nil {
		t.Fatal("expected a mismatch error for the wrong password")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	_, err := HashPassword("short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestHashPasswordTooLong(t *testing.T) {
	// 73 bytes, exceeding bcrypt's 72-byte limit.
	long := strings.Repeat("a", MaxPasswordBytes+1)
	_, err := HashPassword(long)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong, got %v", err)
	}
}
