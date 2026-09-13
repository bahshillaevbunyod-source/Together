BEGIN;

CREATE TABLE posts (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content    text        NOT NULL,
    visibility text        NOT NULL DEFAULT 'public',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT posts_visibility_valid
        CHECK (visibility IN ('public', 'followers', 'private')),

    -- no media table yet, so a post must have non-empty text content
    CONSTRAINT posts_not_empty
        CHECK (length(trim(content)) > 0)
);

CREATE INDEX posts_author_id_created_at_idx ON posts (author_id, created_at DESC);

COMMIT;
