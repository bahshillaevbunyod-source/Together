BEGIN;

CREATE TABLE notifications (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    actor_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    type       text        NOT NULL,
    post_id    uuid        REFERENCES posts (id) ON DELETE CASCADE,
    comment_id uuid        REFERENCES post_comments (id) ON DELETE CASCADE,
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT notifications_type_valid
        CHECK (type IN ('follow', 'post_like', 'post_comment'))
);

CREATE INDEX notifications_user_id_created_at_idx
    ON notifications (user_id, created_at DESC, id DESC);

-- fast unread lookup / count
CREATE INDEX notifications_unread_idx
    ON notifications (user_id)
    WHERE read_at IS NULL;

COMMIT;
