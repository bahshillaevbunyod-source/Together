BEGIN;

ALTER TABLE users
    DROP COLUMN IF EXISTS preferred_language,
    DROP COLUMN IF EXISTS auto_translate_enabled;

COMMIT;
