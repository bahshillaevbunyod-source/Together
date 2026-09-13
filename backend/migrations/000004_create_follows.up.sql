BEGIN;

CREATE TABLE follows (
    follower_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    following_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (follower_id, following_id),
    CONSTRAINT follows_no_self_follow CHECK (follower_id <> following_id)
);

CREATE INDEX follows_follower_id_idx ON follows (follower_id);
CREATE INDEX follows_following_id_idx ON follows (following_id);

COMMIT;
