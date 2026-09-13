BEGIN;

-- Allow media-only posts: content becomes optional and the non-empty check is
-- removed. (A combined posts+media "not empty" rule can't be a plain CHECK
-- across tables and is intentionally not added here.)
ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_not_empty;
ALTER TABLE posts ALTER COLUMN content DROP NOT NULL;

COMMIT;
