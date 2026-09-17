BEGIN;

ALTER TABLE users
    DROP COLUMN IF EXISTS platform_language;

COMMIT;
