BEGIN;

ALTER TABLE users
    ADD COLUMN platform_language text;

UPDATE users
SET platform_language = preferred_language
WHERE preferred_language IS NOT NULL;

COMMIT;
