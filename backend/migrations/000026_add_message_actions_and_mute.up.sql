BEGIN;

ALTER TABLE messages
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN deleted_at timestamptz;

ALTER TABLE conversation_participants
    ADD COLUMN muted_at timestamptz;

CREATE INDEX messages_search_active_idx
    ON messages (conversation_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

COMMIT;
