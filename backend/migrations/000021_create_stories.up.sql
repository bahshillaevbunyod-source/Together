BEGIN;

-- Ephemeral stories. A story is "active" for 24 hours after created_at; that is
-- a query-time rule (see repository), NOT a stored/expired flag, and there is no
-- cleanup job — every active path filters on created_at. The media reference
-- (type/storage_key/mime_type + optional dimensions) mirrors post_media so the
-- later R2 delivery layer can reuse the same presign/URL logic.
CREATE TABLE stories (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type        text        NOT NULL,
    storage_key text        NOT NULL,
    mime_type   text        NOT NULL,
    width       integer,
    height      integer,
    duration_ms bigint,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT stories_type_valid      CHECK (type IN ('image', 'video')),
    CONSTRAINT stories_width_pos       CHECK (width IS NULL OR width > 0),
    CONSTRAINT stories_height_pos      CHECK (height IS NULL OR height > 0),
    CONSTRAINT stories_duration_nonneg CHECK (duration_ms IS NULL OR duration_ms >= 0)
);

-- Author lookup + per-author active/chronological queries.
CREATE INDEX stories_author_id_created_at_idx ON stories (author_id, created_at DESC);
-- Chronological scans across the viewer's followed authors (feed).
CREATE INDEX stories_created_at_idx ON stories (created_at DESC);

COMMIT;
