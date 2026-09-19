BEGIN;

-- Widen the allowed notification types to include 'follow_request', so a
-- private account can be notified of an incoming request. Additive: existing
-- rows already satisfy the wider CHECK.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_type_valid;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_valid
        CHECK (type IN ('follow', 'post_like', 'post_comment', 'follow_request'));

COMMIT;
