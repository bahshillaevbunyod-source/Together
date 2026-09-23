BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM message_attachments) THEN
        RAISE EXCEPTION 'cannot rollback 000027: message attachments exist';
    END IF;
END $$;

DROP TABLE message_attachments;

COMMIT;
