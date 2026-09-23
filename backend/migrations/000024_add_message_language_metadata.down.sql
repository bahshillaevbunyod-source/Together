BEGIN;

DROP INDEX IF EXISTS messages_conversation_sender_created_at_idx;
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_source_language_confidence_check, DROP CONSTRAINT IF EXISTS messages_source_language_metadata_check, DROP COLUMN IF EXISTS source_language_resolution, DROP COLUMN IF EXISTS source_language_confidence, DROP COLUMN IF EXISTS source_language;

COMMIT;
