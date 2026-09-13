BEGIN;

CREATE TABLE post_bookmarks (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id    uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, post_id)
);

-- list a user's bookmarks newest-first
CREATE INDEX post_bookmarks_user_id_created_at_idx
    ON post_bookmarks (user_id, created_at DESC);

COMMIT;
