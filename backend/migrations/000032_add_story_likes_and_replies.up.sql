BEGIN;

-- One like per (story, user). The repository only inserts for an active story
-- whose author is not the liker, idempotently (ON CONFLICT DO NOTHING).
CREATE TABLE story_likes (
    story_id   uuid        NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (story_id, user_id)
);

CREATE INDEX story_likes_user_id_idx ON story_likes (user_id);

-- Links a direct message to the story it replied to. The reply itself is an
-- ordinary message (existing conversation, blocks and realtime apply); this row
-- only adds context. story_id becomes NULL when the story is deleted so the
-- message keeps its "replied to a story" label without pointing anywhere.
CREATE TABLE story_replies (
    message_id uuid        PRIMARY KEY REFERENCES messages (id) ON DELETE CASCADE,
    story_id   uuid        REFERENCES stories (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX story_replies_story_id_idx ON story_replies (story_id);

-- Owner viewer list: newest views first with a deterministic keyset tiebreak.
CREATE INDEX story_views_story_viewed_idx ON story_views (story_id, viewed_at DESC, viewer_id DESC);

COMMIT;
