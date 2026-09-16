package server

import (
	"strings"
	"unicode/utf8"
)

// normalizeDisplayName and normalizeNativeLanguage keep registration and
// profile updates on the same user-facing validation contract.
func normalizeDisplayName(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(value)
	return value, n >= 1 && n <= 80
}

func normalizeNativeLanguage(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(value)
	return value, n >= 2 && n <= 16
}
