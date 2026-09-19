BEGIN;

ALTER TABLE users
    DROP COLUMN is_private;

COMMIT;
