BEGIN;

CREATE TABLE post_comments (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id    uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT post_comments_not_empty CHECK (length(trim(content)) > 0)
);

CREATE INDEX post_comments_post_id_idx
    ON post_comments (post_id, created_at DESC, id DESC);

COMMIT;
