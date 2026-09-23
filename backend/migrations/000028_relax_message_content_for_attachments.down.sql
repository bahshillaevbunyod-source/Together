BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM messages m
        JOIN message_attachments a ON a.message_id = m.id
        WHERE length(btrim(m.content)) = 0
    ) THEN
        RAISE EXCEPTION 'cannot rollback 000028: attachment-only messages exist';
    END IF;
END $$;

ALTER TABLE messages DROP CONSTRAINT messages_content_not_empty;
ALTER TABLE messages ADD CONSTRAINT messages_content_not_empty CHECK (length(btrim(content)) > 0);

COMMIT;
