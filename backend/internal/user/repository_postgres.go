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
          preferred_language, auto_translate_enabled
`

// Create inserts a new user and returns the stored row (without password_hash).
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*User, error) {
	row := r.pool.QueryRow(ctx, insertUserQuery,
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
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
	); err != nil {
		return nil, mapCreateError(err)
	}

	return &u, nil
}

const getUserByEmailQuery = `
SELECT id, email, phone, username, display_name, avatar_url, bio,
       country_code, city, native_language, password_hash, created_at, updated_at,
       preferred_language, auto_translate_enabled
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
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
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
       preferred_language, auto_translate_enabled
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
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
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
       preferred_language, auto_translate_enabled
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
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// profileReturningColumns are the safe columns returned after an update
// (password_hash is intentionally excluded).
const profileReturningColumns = `id, email, phone, username, display_name,
	avatar_url, bio, country_code, city, native_language, created_at, updated_at,
	preferred_language, auto_translate_enabled`

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
	if in.AutoTranslateEnabled != nil {
		add("auto_translate_enabled", *in.AutoTranslateEnabled)
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
		&u.PreferredLanguage,
		&u.AutoTranslateEnabled,
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
