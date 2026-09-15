package server

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"together/backend/internal/translation"
	"together/backend/internal/user"
)

const maxProfileBodyBytes = 1 << 20 // 1 MiB

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// editableProfileFields is the whitelist of keys a PATCH may contain.
var editableProfileFields = map[string]bool{
	"displayName":          true,
	"bio":                  true,
	"countryCode":          true,
	"city":                 true,
	"nativeLanguage":       true,
	"avatarUrl":            true,
	"preferredLanguage":    true,
	"autoTranslateEnabled": true,
}

// profileResponse is the safe profile shape (never password_hash).
type profileResponse struct {
	ID             string  `json:"id"`
	Email          *string `json:"email"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	Bio            *string `json:"bio"`
	CountryCode    *string `json:"countryCode"`
	City           *string `json:"city"`
	NativeLanguage string  `json:"nativeLanguage"`
	AvatarURL      *string `json:"avatarUrl"`
	CreatedAt      string  `json:"createdAt"`

	PreferredLanguage    *string `json:"preferredLanguage"`
	AutoTranslateEnabled bool    `json:"autoTranslateEnabled"`
}

func toProfileResponse(u *user.User) profileResponse {
	return profileResponse{
		ID:                   u.ID,
		Email:                u.Email,
		Username:             u.Username,
		DisplayName:          u.DisplayName,
		Bio:                  u.Bio,
		CountryCode:          u.CountryCode,
		City:                 u.City,
		NativeLanguage:       u.NativeLanguage,
		AvatarURL:            u.AvatarURL,
		CreatedAt:            u.CreatedAt.Format(time.RFC3339),
		PreferredLanguage:    u.PreferredLanguage,
		AutoTranslateEnabled: u.AutoTranslateEnabled,
	}
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	writeJSON(w, http.StatusOK, toProfileResponse(u))
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBodyBytes)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	for key := range raw {
		if !editableProfileFields[key] {
			writeError(w, http.StatusBadRequest, "field not editable: "+key)
			return
		}
	}

	update, ok := buildProfileUpdate(raw)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid profile fields")
		return
	}
	if !update.HasChanges() {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	// Remember the previous avatar (if the caller is changing it) so we can clean
	// up the old object only after the DB update commits.
	_, avatarChanged := raw["avatarUrl"]
	previousAvatar := u.AvatarURL

	updated, err := s.users.UpdateProfile(r.Context(), u.ID, update)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Best-effort: delete the user's PREVIOUS managed avatar object now that the
	// new value is persisted. Only touches this user's own /avatars/ objects and
	// only when the avatar actually changed.
	if avatarChanged {
		s.cleanupPreviousAvatar(r.Context(), u.ID, previousAvatar, updated.AvatarURL)
	}

	writeJSON(w, http.StatusOK, toProfileResponse(updated))
}

// cleanupPreviousAvatar best-effort deletes the user's previous avatar object
// from storage after a profile update. It is deliberately conservative: it only
// deletes an object that (a) is one of ours (public-URL prefix matches), (b)
// lives under this user's own users/{id}/avatars/ namespace, and (c) differs
// from the new avatar. It never deletes legacy /uploads/ avatars, external URLs,
// or other users' objects, and never runs before the DB update has committed.
// Delete failures are ignored: the database is the source of truth.
func (s *Server) cleanupPreviousAvatar(ctx context.Context, userID string, oldURL, newURL *string) {
	if oldURL == nil || *oldURL == "" {
		return
	}
	oldKey := storageKeyFromURL(s.cfg.MediaPublicBaseURL, *oldURL)
	if oldKey == "" {
		return // external URL, not managed by us
	}
	avatarPrefix := "users/" + userID + "/avatars/"
	if !strings.HasPrefix(oldKey, avatarPrefix) {
		return // legacy /uploads/ avatar, another user's key, or unexpected shape
	}
	if strings.Contains(oldKey, "..") || strings.Contains(oldKey, `\`) {
		return
	}
	rest := oldKey[len(avatarPrefix):]
	if rest == "" || strings.Contains(rest, "/") {
		return // must be a single file segment
	}

	newKey := ""
	if newURL != nil {
		newKey = storageKeyFromURL(s.cfg.MediaPublicBaseURL, *newURL)
	}
	if oldKey == newKey {
		return // unchanged; keep the object
	}

	_ = s.storage.DeleteObject(ctx, oldKey) // best-effort
}

// buildProfileUpdate validates raw fields into a ProfileUpdate. ok is false on
// any invalid value.
func buildProfileUpdate(raw map[string]json.RawMessage) (user.ProfileUpdate, bool) {
	var up user.ProfileUpdate

	if v, present := raw["displayName"]; present {
		s, isNull, err := decodeString(v)
		if err != nil || isNull {
			return up, false
		}
		s = strings.TrimSpace(s)
		if n := utf8.RuneCountInString(s); n < 1 || n > 80 {
			return up, false
		}
		up.DisplayName = &s
	}

	if v, present := raw["nativeLanguage"]; present {
		s, isNull, err := decodeString(v)
		if err != nil || isNull {
			return up, false
		}
		s = strings.TrimSpace(s)
		if n := utf8.RuneCountInString(s); n < 2 || n > 16 {
			return up, false
		}
		up.NativeLanguage = &s
	}

	if v, present := raw["bio"]; present {
		opt, ok := decodeNullable(v, 300, nil)
		if !ok {
			return up, false
		}
		up.Bio = opt
	}

	if v, present := raw["city"]; present {
		opt, ok := decodeNullable(v, 100, nil)
		if !ok {
			return up, false
		}
		up.City = opt
	}

	if v, present := raw["avatarUrl"]; present {
		opt, ok := decodeNullable(v, 2048, nil)
		if !ok {
			return up, false
		}
		up.AvatarURL = opt
	}

	if v, present := raw["countryCode"]; present {
		opt, ok := decodeNullable(v, 2, func(s string) bool {
			return countryCodePattern.MatchString(s)
		})
		if !ok {
			return up, false
		}
		up.CountryCode = opt
	}

	if v, present := raw["preferredLanguage"]; present {
		// Nullable: null / "" clears it (fall back to native). A value must be a
		// valid language code by the translation package's rules.
		opt, ok := decodeNullable(v, 16, translation.ValidLanguage)
		if !ok {
			return up, false
		}
		up.PreferredLanguage = opt
	}

	if v, present := raw["autoTranslateEnabled"]; present {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return up, false
		}
		up.AutoTranslateEnabled = &b
	}

	return up, true
}

// decodeString parses a JSON value as a string. isNull is true for JSON null.
func decodeString(raw json.RawMessage) (value string, isNull bool, err error) {
	if string(raw) == "null" {
		return "", true, nil
	}
	err = json.Unmarshal(raw, &value)
	return value, false, err
}

// decodeNullable parses a nullable string field. JSON null (or an empty string
// after trim) clears the field. A non-empty value is trimmed, length-checked
// against maxRunes, and validated by the optional validate func.
func decodeNullable(raw json.RawMessage, maxRunes int, validate func(string) bool) (user.OptionalString, bool) {
	s, isNull, err := decodeString(raw)
	if err != nil {
		return user.OptionalString{}, false
	}
	if isNull {
		return user.OptionalString{Set: true, Value: nil}, true
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return user.OptionalString{Set: true, Value: nil}, true
	}
	if utf8.RuneCountInString(s) > maxRunes {
		return user.OptionalString{}, false
	}
	if validate != nil && !validate(s) {
		return user.OptionalString{}, false
	}
	return user.OptionalString{Set: true, Value: &s}, true
}
