BEGIN;

ALTER TABLE message_attachments DROP CONSTRAINT message_attachments_type_valid;
ALTER TABLE message_attachments ADD CONSTRAINT message_attachments_type_valid CHECK (type IN ('image', 'file', 'voice'));

COMMIT;
