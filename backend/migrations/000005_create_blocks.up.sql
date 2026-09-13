BEGIN;

CREATE TABLE blocks (
    blocker_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (blocker_id, blocked_id),
    CONSTRAINT blocks_no_self_block CHECK (blocker_id <> blocked_id)
);

CREATE INDEX blocks_blocker_id_idx ON blocks (blocker_id);
CREATE INDEX blocks_blocked_id_idx ON blocks (blocked_id);

COMMIT;
