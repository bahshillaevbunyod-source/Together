BEGIN;

ALTER TABLE messages DROP CONSTRAINT messages_content_not_empty;
ALTER TABLE messages ADD CONSTRAINT messages_content_not_empty CHECK (length(btrim(content)) >= 0);

COMMIT;
