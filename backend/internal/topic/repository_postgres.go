package topic

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
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
