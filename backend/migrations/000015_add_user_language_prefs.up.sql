BEGIN;

-- Per-user translation preferences. preferred_language is nullable (NULL means
-- "fall back to the user's native language"); auto_translate defaults off.
ALTER TABLE users
    ADD COLUMN preferred_language     text,
    ADD COLUMN auto_translate_enabled boolean NOT NULL DEFAULT false;

COMMIT;
