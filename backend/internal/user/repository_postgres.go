package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the production Repository backed by pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// Compile-time assurance that the production repository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a Repository over the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const insertUserQuery = `
INSERT INTO users (email, username, display_name, native_language, password_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, phone, username, display_name, avatar_url, bio,
          country_code, city, native_language, created_at, updated_at,
          platform_language, preferred_language, auto_translate_enabled, is_private
`

// Create inserts a new user and returns the stored row (without password_hash).
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*User, error) {
	return createUser(ctx, r.pool, in)
}

// CreateTx creates a user through a caller-owned transaction. It is used for
// registration so the account and its first session either both commit or both
// roll back.
func (r *PostgresRepository) CreateTx(ctx context.Context, q DBTX, in CreateInput) (*User, error) {
	return createUser(ctx, q, in)
}

func createUser(ctx context.Context, q DBTX, in CreateInput) (*User, error) {
	row := q.QueryRow(ctx, insertUserQuery,
		in.Email, in.Username, in.DisplayName, in.NativeLanguage, in.PasswordHash,
	)

	var u User
	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Phone,
		&u.Username,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.CountryCode,
		&u.City,
		&u.NativeLanguage,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.PlatformLanguage,
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
		&u.IsPrivate,
	); err != nil {
		return nil, mapCreateError(err)
	}

	return &u, nil
}

const getUserByEmailQuery = `
SELECT id, email, phone, username, display_name, avatar_url, bio,
       country_code, city, native_language, password_hash, created_at, updated_at,
       platform_language, preferred_language, auto_translate_enabled, is_private
FROM users
WHERE email = $1
`

// GetByEmail loads a user (including PasswordHash) by exact email match.
// Returns ErrNotFound when no row exists.
func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	row := r.pool.QueryRow(ctx, getUserByEmailQuery, email)

	var u User
	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Phone,
		&u.Username,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.CountryCode,
		&u.City,
		&u.NativeLanguage,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.PlatformLanguage,
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
		&u.IsPrivate,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &u, nil
}

const getUserByIDQuery = `
SELECT id, email, phone, username, display_name, avatar_url, bio,
       country_code, city, native_language, password_hash, created_at, updated_at,
       platform_language, preferred_language, auto_translate_enabled, is_private
FROM users
WHERE id = $1
`

// GetByID loads a user by primary key. Returns ErrNotFound when no row exists.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*User, error) {
	row := r.pool.QueryRow(ctx, getUserByIDQuery, id)

	var u User
	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Phone,
		&u.Username,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.CountryCode,
		&u.City,
		&u.NativeLanguage,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.PlatformLanguage,
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
		&u.IsPrivate,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

const getUserByUsernameQuery = `
SELECT id, email, phone, username, display_name, avatar_url, bio,
       country_code, city, native_language, password_hash, created_at, updated_at,
       platform_language, preferred_language, auto_translate_enabled, is_private
FROM users
WHERE username = $1
`

// GetByUsername loads a user by username. Returns ErrNotFound when no row
// exists.
func (r *PostgresRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	row := r.pool.QueryRow(ctx, getUserByUsernameQuery, username)

	var u User
	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Phone,
		&u.Username,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.CountryCode,
		&u.City,
		&u.NativeLanguage,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.PlatformLanguage,
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
		&u.IsPrivate,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// escapeLike escapes LIKE metacharacters (\ % _) so user input is matched
// literally. Pairs with the ESCAPE '\' clause in searchUsersQuery.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// searchUsersQuery finds users matching a query on username or display name,
// case-insensitively (usernames are stored lowercase; display_name is lowered
// in-query). It excludes the viewer ($1) and any user in a block relationship
// with them (either direction). Ranking: exact username, username prefix,
// display-name prefix, then any contains match; ties break on username then id
// for stable, deterministic ordering. Fully parameterized.
//
//	$1 = viewer id, $2 = normalized query (trimmed, lowercased),
//	$3 = $2 with LIKE metacharacters escaped, $4 = limit.
const searchUsersQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url
FROM users u
WHERE u.id <> $1
  AND (
    u.username LIKE '%' || $3 || '%' ESCAPE '\'
    OR lower(u.display_name) LIKE '%' || $3 || '%' ESCAPE '\'
  )
  AND NOT EXISTS (
    SELECT 1 FROM blocks bl
    WHERE (bl.blocker_id = $1 AND bl.blocked_id = u.id)
       OR (bl.blocker_id = u.id AND bl.blocked_id = $1)
  )
ORDER BY
  CASE
    WHEN u.username = $2 THEN 0
    WHEN u.username LIKE $3 || '%' ESCAPE '\' THEN 1
    WHEN lower(u.display_name) LIKE $3 || '%' ESCAPE '\' THEN 2
    ELSE 3
  END,
  u.username,
  u.id
LIMIT $4
`

// SearchUsers implements Repository.SearchUsers.
func (r *PostgresRepository) SearchUsers(ctx context.Context, viewerID, query string, limit int) ([]SearchResult, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	rows, err := r.pool.Query(ctx, searchUsersQuery, viewerID, q, escapeLike(q), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var s SearchResult
		if err := rows.Scan(&s.ID, &s.Username, &s.DisplayName, &s.AvatarURL); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// discoverCandidate is the shared inner SELECT: it excludes the viewer ($1), any
// block relationship (either direction), users the viewer already follows, and
// users with an outgoing pending request from the viewer. It computes a real
// follower count per candidate. $2 = viewer country, $3 = viewer language, $4 =
// optional world country filter (empty string = no filter).
// The mode-specific outer query adds keyset predicates and ORDER BY.
const discoverCandidate = `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.country_code, u.city,
       u.native_language, u.created_at,
       (SELECT count(*) FROM follows f WHERE f.following_id = u.id) AS follower_count
FROM users u
WHERE u.id <> $1
  AND NOT EXISTS (
    SELECT 1 FROM blocks bl
    WHERE (bl.blocker_id = $1 AND bl.blocked_id = u.id)
       OR (bl.blocker_id = u.id AND bl.blocked_id = $1)
  )
  AND NOT EXISTS (
    SELECT 1 FROM follows fol
    WHERE fol.follower_id = $1 AND fol.following_id = u.id
  )
  AND NOT EXISTS (
    SELECT 1 FROM follow_requests fr
    WHERE fr.requester_id = $1 AND fr.target_id = u.id
  )`

// The 11 output columns, in the order every mode selects and Scan reads them.
// id, username, display_name, avatar_url, country_code, city, native_language,
// created_at, follower_count, score, country_rank.

// DiscoverUsers implements Repository.DiscoverUsers with opaque keyset
// pagination. Ordering is deterministic per mode (id is always the final
// tie-break), so pages never duplicate or skip rows.
func (r *PostgresRepository) DiscoverUsers(ctx context.Context, p DiscoverParams) ([]DiscoverResult, error) {
	var query string
	var args []any

	switch p.Mode {
	case DiscoverPopular:
		// key: (follower_count DESC, id DESC)
		var fc, id any
		if p.After != nil {
			fc, id = p.After.FollowerCount, p.After.ID
		}
		query = fmt.Sprintf(`
SELECT c.id, c.username, c.display_name, c.avatar_url, c.country_code, c.city,
       c.native_language, c.created_at, c.follower_count, 0 AS score, 0 AS country_rank
FROM (%s) c
WHERE ($2::bigint IS NULL
       OR c.follower_count < $2
       OR (c.follower_count = $2 AND c.id < $3::uuid))
ORDER BY c.follower_count DESC, c.id DESC
LIMIT $4`, discoverCandidate)
		args = []any{p.ViewerID, fc, id, p.Limit}

	case DiscoverWorld:
		// key: (country_rank ASC, follower_count DESC, id DESC)
		var rank, fc, id any
		if p.After != nil {
			rank, fc, id = p.After.CountryRank, p.After.FollowerCount, p.After.ID
		}
		// Placeholders are contiguous ($1..$7); world does not use viewer language,
		// so it is not passed (a gap would make Postgres fail to type the param).
		//   $1 viewer id, $2 viewer country, $3 country filter,
		//   $4 rank key, $5 follower_count key, $6 id key, $7 limit.
		query = fmt.Sprintf(`
SELECT c.id, c.username, c.display_name, c.avatar_url, c.country_code, c.city,
       c.native_language, c.created_at, c.follower_count, 0 AS score,
       (CASE WHEN c.country_code IS NULL THEN 2
             WHEN c.country_code = $2 THEN 1
             ELSE 0 END) AS country_rank
FROM (%s) c
WHERE ($3 = '' OR c.country_code = $3)
  AND ($4::int IS NULL
       OR (CASE WHEN c.country_code IS NULL THEN 2
                WHEN c.country_code = $2 THEN 1 ELSE 0 END) > $4
       OR ((CASE WHEN c.country_code IS NULL THEN 2
                 WHEN c.country_code = $2 THEN 1 ELSE 0 END) = $4 AND c.follower_count < $5)
       OR ((CASE WHEN c.country_code IS NULL THEN 2
                 WHEN c.country_code = $2 THEN 1 ELSE 0 END) = $4 AND c.follower_count = $5 AND c.id < $6::uuid))
ORDER BY country_rank ASC, c.follower_count DESC, c.id DESC
LIMIT $7`, discoverCandidate)
		args = []any{p.ViewerID, p.ViewerCountry, p.Country, rank, fc, id, p.Limit}

	case DiscoverForYou, "":
		// key: (score DESC, created_at DESC, id DESC), where
		// score = follower_count + light country/language boosts.
		var score, created, id any
		if p.After != nil {
			score, created, id = p.After.Score, p.After.CreatedAt, p.After.ID
		}
		query = fmt.Sprintf(`
SELECT s.id, s.username, s.display_name, s.avatar_url, s.country_code, s.city,
       s.native_language, s.created_at, s.follower_count, s.score, 0 AS country_rank
FROM (
  SELECT c.*,
         (c.follower_count
          + CASE WHEN c.country_code = $2 THEN %d ELSE 0 END
          + CASE WHEN c.native_language = $3 THEN %d ELSE 0 END) AS score
  FROM (%s) c
) s
WHERE ($4::bigint IS NULL
       OR s.score < $4
       OR (s.score = $4 AND s.created_at < $5::timestamptz)
       OR (s.score = $4 AND s.created_at = $5 AND s.id < $6::uuid))
ORDER BY s.score DESC, s.created_at DESC, s.id DESC
LIMIT $7`, ForYouCountryBoost, ForYouLanguageBoost, discoverCandidate)
		args = []any{p.ViewerID, p.ViewerCountry, p.ViewerLanguage, score, created, id, p.Limit}

	default:
		return nil, fmt.Errorf("unknown discover mode %q", p.Mode)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiscoverResult
	for rows.Next() {
		var d DiscoverResult
		if err := rows.Scan(
			&d.ID, &d.Username, &d.DisplayName, &d.AvatarURL,
			&d.CountryCode, &d.City, &d.NativeLanguage, &d.CreatedAt,
			&d.FollowerCount, &d.Score, &d.CountryRank,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// profileReturningColumns are the safe columns returned after an update
// (password_hash is intentionally excluded).
const profileReturningColumns = `id, email, phone, username, display_name,
	avatar_url, bio, country_code, city, native_language, created_at, updated_at,
	platform_language, preferred_language, auto_translate_enabled, is_private`

// UpdateProfile applies a partial update to the editable profile columns and
// returns the updated row. Only provided fields are written. Returns
// ErrNotFound when the user does not exist.
func (r *PostgresRepository) UpdateProfile(ctx context.Context, id string, in ProfileUpdate) (*User, error) {
	set := make([]string, 0, 7)
	args := make([]any, 0, 8)
	i := 1
	add := func(col string, val any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	if in.DisplayName != nil {
		add("display_name", *in.DisplayName)
	}
	if in.NativeLanguage != nil {
		add("native_language", *in.NativeLanguage)
	}
	if in.Bio.Set {
		add("bio", in.Bio.Value)
	}
	if in.CountryCode.Set {
		add("country_code", in.CountryCode.Value)
	}
	if in.City.Set {
		add("city", in.City.Value)
	}
	if in.AvatarURL.Set {
		add("avatar_url", in.AvatarURL.Value)
	}
	if in.PreferredLanguage.Set {
		add("preferred_language", in.PreferredLanguage.Value)
	}
	if in.PlatformLanguage.Set {
		add("platform_language", in.PlatformLanguage.Value)
	}
	if in.AutoTranslateEnabled != nil {
		add("auto_translate_enabled", *in.AutoTranslateEnabled)
	}
	if in.IsPrivate != nil {
		add("is_private", *in.IsPrivate)
	}
	set = append(set, "updated_at = now()")

	query := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = $%d RETURNING %s",
		strings.Join(set, ", "), i, profileReturningColumns,
	)
	args = append(args, id)

	var u User
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&u.ID,
		&u.Email,
		&u.Phone,
		&u.Username,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.CountryCode,
		&u.City,
		&u.NativeLanguage,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.PlatformLanguage,
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
		&u.IsPrivate,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// mapCreateError translates unique-violation errors into domain errors and
// leaves everything else opaque (no SQL/credentials leaked upstream).
func mapCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		switch pgErr.ConstraintName {
		case "users_email_key":
			return ErrDuplicateEmail
		case "users_username_key":
			return ErrDuplicateUsername
		}
	}
	return err
}
