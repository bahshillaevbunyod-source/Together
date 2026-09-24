BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM conversations WHERE type <> 'direct') OR
       EXISTS (SELECT 1 FROM conversation_participants WHERE role <> 'member') THEN
        RAISE EXCEPTION 'cannot rollback 000030: community data exists';
    END IF;
END $$;

DROP INDEX conversation_participants_conversation_role_idx;
DROP INDEX conversations_type_updated_idx;
ALTER TABLE conversation_participants DROP CONSTRAINT conversation_participants_role_valid;
ALTER TABLE conversation_participants DROP COLUMN role;
ALTER TABLE conversations DROP CONSTRAINT conversations_type_shape;
ALTER TABLE conversations DROP COLUMN avatar_url, DROP COLUMN description, DROP COLUMN name, DROP COLUMN type;
ALTER TABLE conversations ALTER COLUMN user_low SET NOT NULL, ALTER COLUMN user_high SET NOT NULL;
ALTER TABLE conversations ADD CONSTRAINT conversations_pair_ordered CHECK (user_low < user_high);

COMMIT;
