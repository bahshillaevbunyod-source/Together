BEGIN;

CREATE TABLE message_attachments (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id      uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    storage_key     text        NOT NULL UNIQUE,
    filename        text        NOT NULL,
    type            text        NOT NULL,
    mime_type       text        NOT NULL,
    size_bytes      bigint      NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT message_attachments_type_valid CHECK (type IN ('image', 'file')),
    CONSTRAINT message_attachments_filename_nonempty CHECK (length(btrim(filename)) > 0),
    CONSTRAINT message_attachments_size_pos CHECK (size_bytes > 0),
    CONSTRAINT message_attachments_one_per_message UNIQUE (message_id)
);

CREATE INDEX message_attachments_message_id_idx ON message_attachments (message_id);

COMMIT;
