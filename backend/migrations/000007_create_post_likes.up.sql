BEGIN;

CREATE TABLE post_likes (
    post_id    uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (post_id, user_id)
);

CREATE INDEX post_likes_user_id_idx ON post_likes (user_id);

COMMIT;
