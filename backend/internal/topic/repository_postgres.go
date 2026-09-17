package topic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/post"
)

// PostgresRepository is the PostgreSQL-backed topic repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a topic repository over the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const ensureTopicQuery = `
INSERT INTO topics (slug)
VALUES ($1)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id
`

const linkPostTopicQuery = `
INSERT INTO post_topics (post_id, topic_id)
VALUES ($1, $2)
ON CONFLICT (post_id, topic_id) DO NOTHING
`

// CreatePostTopicsTx ensures each slug exists and links it to the post.
// The caller owns the transaction and decides whether to commit or roll back.
func (r *PostgresRepository) CreatePostTopicsTx(ctx context.Context, q DBTX, postID string, slugs []string) error {
	for _, slug := range slugs {
		var topicID string
		if err := q.QueryRow(ctx, ensureTopicQuery, slug).Scan(&topicID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, linkPostTopicQuery, postID, topicID); err != nil {
			return err
		}
	}
	return nil
}

const getBySlugQuery = `
SELECT id, slug, created_at
FROM topics
WHERE slug = $1
`

// GetBySlug returns the canonical topic row for slug.
func (r *PostgresRepository) GetBySlug(ctx context.Context, slug string) (*Topic, error) {
	var t Topic
	if err := r.pool.QueryRow(ctx, getBySlugQuery, slug).Scan(&t.ID, &t.Slug, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

// ListTrending returns topics ordered by posts visible to viewer. The same
// visibility, follow, and block predicates as the feed keep private topic use
// from affecting an ineligible viewer's result.
const listTrendingQuery = `
SELECT t.slug, count(*) AS posts_count
FROM topics t
JOIN post_topics pt ON pt.topic_id = t.id
JOIN posts p ON p.id = pt.post_id
WHERE (
        p.visibility = 'public'
        OR ($1::uuid IS NOT NULL AND p.author_id = $1)
        OR ($1::uuid IS NOT NULL AND ` + post.SQLFollowsAuthor + ` AND p.visibility = 'followers')
      )
  AND ($1::uuid IS NULL OR ` + post.SQLNotBlocked + `)
GROUP BY t.id, t.slug
ORDER BY posts_count DESC, t.slug ASC
LIMIT $2
`

func (r *PostgresRepository) ListTrending(ctx context.Context, viewerID *string, limit int) ([]TrendingItem, error) {
	var viewer any
	if viewerID != nil {
		viewer = *viewerID
	}

	rows, err := r.pool.Query(ctx, listTrendingQuery, viewer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TrendingItem, 0, limit)
	for rows.Next() {
		var item TrendingItem
		if err := rows.Scan(&item.Slug, &item.PostsCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// searchTopicsQuery uses position rather than LIKE so underscore, which is a
// valid slug character, remains a literal search character. Visibility and
// block conditions intentionally match ListTrending.
const searchTopicsQuery = `
SELECT t.slug, count(*) AS posts_count
FROM topics t
JOIN post_topics pt ON pt.topic_id = t.id
JOIN posts p ON p.id = pt.post_id
WHERE position($2 in t.slug) > 0
  AND (
        p.visibility = 'public'
        OR ($1::uuid IS NOT NULL AND p.author_id = $1)
        OR ($1::uuid IS NOT NULL AND ` + post.SQLFollowsAuthor + ` AND p.visibility = 'followers')
      )
  AND ($1::uuid IS NULL OR ` + post.SQLNotBlocked + `)
GROUP BY t.id, t.slug
ORDER BY CASE WHEN t.slug = $2 THEN 0 ELSE 1 END, posts_count DESC, t.slug ASC
LIMIT $3
`

// Search returns matching canonical slugs with counts for posts visible to
// viewer. The caller validates and normalizes query before this repository
// method is invoked.
func (r *PostgresRepository) Search(ctx context.Context, viewerID *string, query string, limit int) ([]TrendingItem, error) {
	var viewer any
	if viewerID != nil {
		viewer = *viewerID
	}

	rows, err := r.pool.Query(ctx, searchTopicsQuery, viewer, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TrendingItem, 0, limit)
	for rows.Next() {
		var item TrendingItem
		if err := rows.Scan(&item.Slug, &item.PostsCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
