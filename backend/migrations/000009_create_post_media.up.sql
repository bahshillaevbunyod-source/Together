BEGIN;

CREATE TABLE post_media (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id     uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    type        text        NOT NULL,
    storage_key text        NOT NULL,
    mime_type   text        NOT NULL,
    size_bytes  bigint      NOT NULL,
    width       integer,
    height      integer,
    duration_ms bigint,
    sort_order  integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT post_media_type_valid   CHECK (type IN ('image', 'video')),
    CONSTRAINT post_media_size_pos      CHECK (size_bytes > 0),
    CONSTRAINT post_media_sort_nonneg   CHECK (sort_order >= 0),
    CONSTRAINT post_media_width_pos      CHECK (width IS NULL OR width > 0),
    CONSTRAINT post_media_height_pos     CHECK (height IS NULL OR height > 0),
    CONSTRAINT post_media_duration_nonneg CHECK (duration_ms IS NULL OR duration_ms >= 0),
    CONSTRAINT post_media_unique_order   UNIQUE (post_id, sort_order)
);

CREATE INDEX post_media_post_id_sort_idx ON post_media (post_id, sort_order);

COMMIT;
