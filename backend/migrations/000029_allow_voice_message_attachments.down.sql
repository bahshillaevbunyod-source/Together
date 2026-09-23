BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM message_attachments WHERE type = 'voice') THEN
        RAISE EXCEPTION 'cannot rollback 000029: voice attachments exist';
    END IF;
END $$;

ALTER TABLE message_attachments DROP CONSTRAINT message_attachments_type_valid;
ALTER TABLE message_attachments ADD CONSTRAINT message_attachments_type_valid CHECK (type IN ('image', 'file'));

COMMIT;
