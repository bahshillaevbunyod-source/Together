BEGIN;

-- Pending follow requests for private accounts. This is intentionally SEPARATE
-- from `follows`: a row here NEVER grants follower access. Accepting a request
-- deletes its row here and inserts the canonical edge into `follows`; declining
-- or cancelling just deletes the row here.
CREATE TABLE follow_requests (
    requester_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (requester_id, target_id),
    CONSTRAINT follow_requests_no_self CHECK (requester_id <> target_id)
);

CREATE INDEX follow_requests_target_id_idx ON follow_requests (target_id);
CREATE INDEX follow_requests_requester_id_idx ON follow_requests (requester_id);

COMMIT;
