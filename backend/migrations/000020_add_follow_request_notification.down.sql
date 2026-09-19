BEGIN;

-- Restore the original narrower CHECK. Remove any rows using the newer type
-- first so re-adding the constraint cannot fail on existing data.
DELETE FROM notifications WHERE type = 'follow_request';

ALTER TABLE notifications
    DROP CONSTRAINT notifications_type_valid;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_valid
        CHECK (type IN ('follow', 'post_like', 'post_comment'));

COMMIT;
