BEGIN;

DROP INDEX IF EXISTS posts_original_post_id_idx;
DROP INDEX IF EXISTS posts_author_original_post_unique;
ALTER TABLE posts DROP COLUMN IF EXISTS original_post_id;

COMMIT;
