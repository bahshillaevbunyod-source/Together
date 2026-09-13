BEGIN;

-- A given uploaded object (storage_key) may belong to only one post.
ALTER TABLE post_media
    ADD CONSTRAINT post_media_storage_key_unique UNIQUE (storage_key);

COMMIT;
