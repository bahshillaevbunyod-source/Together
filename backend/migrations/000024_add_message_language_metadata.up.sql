BEGIN;

ALTER TABLE messages
    ADD COLUMN source_language text,
    ADD COLUMN source_language_confidence double precision,
    ADD COLUMN source_language_resolution text,
    ADD CONSTRAINT messages_source_language_metadata_check CHECK (
        (source_language_resolution IS NULL AND source_language IS NULL AND source_language_confidence IS NULL)
        OR (source_language_resolution IN ('current', 'context') AND source_language IS NOT NULL AND source_language_confidence IS NOT NULL)
        OR (source_language_resolution = 'unresolved' AND source_language IS NULL AND source_language_confidence IS NULL)
    ),
    ADD CONSTRAINT messages_source_language_confidence_check CHECK (
        source_language_confidence IS NULL OR (source_language_confidence >= 0 AND source_language_confidence <= 1)
    );

CREATE INDEX messages_conversation_sender_created_at_idx ON messages (conversation_id, sender_id, created_at DESC, id DESC);

COMMIT;
