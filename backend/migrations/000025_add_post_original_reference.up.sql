BEGIN;

ALTER TABLE posts
    ADD COLUMN original_post_id uuid REFERENCES posts(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX posts_author_original_post_unique
    ON posts (author_id, original_post_id)
    WHERE original_post_id IS NOT NULL;

CREATE INDEX posts_original_post_id_idx ON posts (original_post_id)
    WHERE original_post_id IS NOT NULL;

COMMIT;
