BEGIN;

ALTER TABLE conversations
    ALTER COLUMN user_low DROP NOT NULL,
    ALTER COLUMN user_high DROP NOT NULL,
    ADD COLUMN type text NOT NULL DEFAULT 'direct',
    ADD COLUMN name text,
    ADD COLUMN description text,
    ADD COLUMN avatar_url text;
ALTER TABLE conversations DROP CONSTRAINT conversations_pair_ordered;
ALTER TABLE conversations ADD CONSTRAINT conversations_type_shape CHECK (
    (type = 'direct' AND user_low IS NOT NULL AND user_high IS NOT NULL AND user_low < user_high AND name IS NULL)
    OR (type IN ('group','channel') AND user_low IS NULL AND user_high IS NULL AND length(btrim(name)) > 0)
);
ALTER TABLE conversation_participants ADD COLUMN role text NOT NULL DEFAULT 'member';
ALTER TABLE conversation_participants ADD CONSTRAINT conversation_participants_role_valid CHECK (role IN ('owner','admin','member'));
CREATE INDEX conversations_type_updated_idx ON conversations (type, updated_at DESC, id DESC);
CREATE INDEX conversation_participants_conversation_role_idx ON conversation_participants (conversation_id, role);

COMMIT;
