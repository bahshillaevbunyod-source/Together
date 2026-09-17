// Package user holds the User domain model. It mirrors the `users` table
// defined in migrations/000001_create_users. No repository/query methods yet.
package user

import "time"

// User mirrors a row in the `users` table. Nullable columns are represented
// as pointers (nil == SQL NULL).
type User struct {
	ID             string    // uuid primary key
	Email          *string   // nullable, unique when present
	Phone          *string   // nullable, unique when present
	Username       string    // unique, stored lowercase
	DisplayName    string    // required
	AvatarURL      *string   // nullable
	Bio            *string   // nullable
	CountryCode    *string   // nullable
	City           *string   // nullable
	NativeLanguage string    // required
	PasswordHash   *string   // nullable bcrypt hash; never plaintext
	CreatedAt      time.Time // set by DB default now()
	UpdatedAt      time.Time // set by DB default now()

	// Language preferences.
	PlatformLanguage     *string // nullable until onboarding; UI and translation target
	PreferredLanguage    *string // nullable; independent translation preference
	AutoTranslateEnabled bool    // default false
}
