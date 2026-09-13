BEGIN;

CREATE TABLE sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 hash of the raw token; the raw token is never stored.
    token_hash text        NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- lookups by user (e.g. revoke all sessions) and expiry cleanup.
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

COMMIT;
