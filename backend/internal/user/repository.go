package user

import (
	"context"
	"errors"
)

// Repository errors the callers may branch on.
var (
	ErrDuplicateEmail    = errors.New("email already in use")
	ErrDuplicateUsername = errors.New("username already in use")
	ErrNotFound          = errors.New("user not found")
)

// CreateInput carries the already-validated, normalized fields needed to
// persist a new user. PasswordHash is a bcrypt hash — never plaintext.
type CreateInput struct {
	Email          string
	Username       string
	DisplayName    string
	NativeLanguage string
	PasswordHash   string
}

// OptionalString marks a nullable field in a partial update. Set indicates the
// field was provided; a nil Value clears the column (SQL NULL).
type OptionalString struct {
	Set   bool
	Value *string
}

// ProfileUpdate is a validated partial update of the editable profile fields.
// A nil pointer / unset Optional means "leave unchanged".
type ProfileUpdate struct {
	DisplayName    *string        // non-null column
	NativeLanguage *string        // non-null column
	Bio            OptionalString // nullable
	CountryCode    OptionalString // nullable
	City           OptionalString // nullable
	AvatarURL      OptionalString // nullable

	PreferredLanguage    OptionalString // nullable; nil clears (fall back to native)
	AutoTranslateEnabled *bool          // non-null column
}

// HasChanges reports whether at least one field will be updated.
func (p ProfileUpdate) HasChanges() bool {
	return p.DisplayName != nil || p.NativeLanguage != nil ||
		p.Bio.Set || p.CountryCode.Set || p.City.Set || p.AvatarURL.Set ||
		p.PreferredLanguage.Set || p.AutoTranslateEnabled != nil
}

// Repository abstracts user persistence so handlers never touch SQL.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*User, error)
	// GetByEmail returns the user (including PasswordHash) or ErrNotFound.
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByID returns the user by id or ErrNotFound.
	GetByID(ctx context.Context, id string) (*User, error)
	// GetByUsername returns the user by username or ErrNotFound.
	GetByUsername(ctx context.Context, username string) (*User, error)
	// UpdateProfile applies a partial profile update and returns the row.
	UpdateProfile(ctx context.Context, id string, in ProfileUpdate) (*User, error)
}
