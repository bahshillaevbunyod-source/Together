BEGIN;

-- A private 1-to-1 conversation between exactly two distinct users.
-- The two members are stored as a canonical ordered pair (user_low < user_high)
-- so that a UNIQUE constraint prevents more than one conversation per pair and
-- the CHECK forbids a conversation with oneself.
CREATE TABLE conversations (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_high  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT conversations_pair_ordered CHECK (user_low < user_high),
    CONSTRAINT conversations_unique_pair UNIQUE (user_low, user_high)
);

-- Find a user's conversations from either side of the pair.
CREATE INDEX conversations_user_low_idx ON conversations (user_low);
CREATE INDEX conversations_user_high_idx ON conversations (user_high);

-- A single message within a conversation.
CREATE TABLE messages (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    sender_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content         text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_content_not_empty CHECK (length(btrim(content)) > 0)
);

-- Keyset pagination of a conversation's messages, newest first.
CREATE INDEX messages_conversation_created_at_idx
    ON messages (conversation_id, created_at DESC, id DESC);

-- Per-participant read-state foundation: one row per (conversation, member).
-- The two rows per conversation mirror the conversation's pair; they carry the
-- read markers used later for unread counts. No delete behavior yet.
CREATE TABLE conversation_participants (
    conversation_id      uuid        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    user_id              uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_message_id uuid        REFERENCES messages (id) ON DELETE SET NULL,
    last_read_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (conversation_id, user_id)
);

-- List a user's conversations via their participant rows.
CREATE INDEX conversation_participants_user_idx
    ON conversation_participants (user_id);

COMMIT;
