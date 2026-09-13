package server

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// encodeCursor builds an opaque keyset cursor from a timestamp and an id.
func encodeCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", createdAt.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses an opaque cursor into (createdAt, id). ok is false when
// the cursor is malformed.
func decodeCursor(raw string) (time.Time, string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", false
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", false
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.Unix(0, nanos).UTC(), parts[1], true
}

// parseListLimit returns the requested limit. Empty -> default. Out of range or
// non-numeric -> ok=false (400).
func parseListLimit(raw string) (int, bool) {
	if raw == "" {
		return defaultListLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxListLimit {
		return 0, false
	}
	return n, true
}
