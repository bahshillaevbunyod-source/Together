BEGIN;

DROP INDEX IF EXISTS messages_search_active_idx;
ALTER TABLE conversation_participants DROP COLUMN muted_at;
ALTER TABLE messages DROP COLUMN deleted_at, DROP COLUMN updated_at;

COMMIT;
