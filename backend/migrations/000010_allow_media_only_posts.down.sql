BEGIN;

ALTER TABLE posts ALTER COLUMN content SET NOT NULL;
ALTER TABLE posts ADD CONSTRAINT posts_not_empty CHECK (length(trim(content)) > 0);

COMMIT;
