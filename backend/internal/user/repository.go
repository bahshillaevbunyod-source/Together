package user

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// DBTX is the minimal query executor shared by a pgx pool and transaction.
// It lets registration create the user inside the same transaction as its
// first session without coupling this package to a service implementation.
type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

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

	PlatformLanguage     OptionalString // nullable; nil clears (onboarding incomplete)
	PreferredLanguage    OptionalString // nullable; independent preference
	AutoTranslateEnabled *bool          // non-null column
}

// HasChanges reports whether at least one field will be updated.
func (p ProfileUpdate) HasChanges() bool {
	return p.DisplayName != nil || p.NativeLanguage != nil ||
		p.Bio.Set || p.CountryCode.Set || p.City.Set || p.AvatarURL.Set ||
		p.PlatformLanguage.Set || p.PreferredLanguage.Set || p.AutoTranslateEnabled != nil
}

// SearchResult is a public-safe user row returned by SearchUsers. It never
// carries private fields (email, phone, password hash).
type SearchResult struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   *string
}

// DiscoverResult is a public-safe user row for the discovery surface. It carries
// the follower count and location/language so the client can render rich cards.
// Score and CountryRank are the mode-specific sort keys used to build the next
// keyset cursor (Score for for_you, CountryRank+FollowerCount for world,
// FollowerCount for popular); CreatedAt is the for_you secondary key.
type DiscoverResult struct {
	ID             string
	Username       string
	DisplayName    string
	AvatarURL      *string
	CountryCode    *string
	City           *string
	NativeLanguage string
	FollowerCount  int64
	CreatedAt      time.Time
	Score          int64
	CountryRank    int
}

// Discover modes.
const (
	DiscoverForYou  = "for_you"
	DiscoverWorld   = "world"
	DiscoverPopular = "popular"
)

// for_you relevance boosts, expressed in follower-equivalent units so they stay
// LIGHT: a same-country user is nudged up by a few followers' worth, a
// same-language user slightly less. They never hard-gate — a popular user from
// another country still outranks a weak same-country user.
const (
	ForYouCountryBoost  = 3
	ForYouLanguageBoost = 2
)

// DiscoverCursor is the decoded keyset position for the next page. Which fields
// matter depends on the mode; ID is always the final tie-break.
type DiscoverCursor struct {
	Score         int64
	FollowerCount int64
	CountryRank   int
	CreatedAt     time.Time
	ID            string
}

// DiscoverParams bundles the inputs for DiscoverUsers. ViewerCountry and
// ViewerLanguage feed the for_you relevance boost and the world ordering; both
// may be empty. Country is an optional exact filter used by the world mode.
// Pagination is keyset-based: After is nil for the first page, otherwise the
// decoded cursor position; Limit is validated by the caller.
type DiscoverParams struct {
	ViewerID       string
	Mode           string
	Country        string
	ViewerCountry  string
	ViewerLanguage string
	After          *DiscoverCursor
	Limit          int
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
	// SearchUsers finds users whose username or display name matches query
	// (case-insensitive), excluding the viewer and anyone in a block
	// relationship with them, ranked by match quality and capped at limit.
	SearchUsers(ctx context.Context, viewerID, query string, limit int) ([]SearchResult, error)
	// DiscoverUsers returns people-discovery results for the given mode,
	// excluding the viewer, blocked relationships (either direction) and users
	// the viewer already follows. Ranking is deterministic per mode.
	DiscoverUsers(ctx context.Context, p DiscoverParams) ([]DiscoverResult, error)
}
