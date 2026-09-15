BEGIN;

CREATE TABLE topics (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT topics_slug_canonical
        CHECK (slug = lower(btrim(slug)) AND length(slug) > 0),
    CONSTRAINT topics_slug_length
        CHECK (char_length(slug) <= 100),
    CONSTRAINT topics_slug_unique UNIQUE (slug)
);

CREATE TABLE post_topics (
    post_id  uuid NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    topic_id uuid NOT NULL REFERENCES topics (id) ON DELETE CASCADE,

    PRIMARY KEY (post_id, topic_id)
);

-- look up posts associated with a topic
CREATE INDEX post_topics_topic_id_post_id_idx
    ON post_topics (topic_id, post_id);

COMMIT;
