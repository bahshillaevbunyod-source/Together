BEGIN;

ALTER TABLE post_media
    DROP CONSTRAINT IF EXISTS post_media_storage_key_unique;

COMMIT;
