BEGIN;

-- Per-viewer viewed/unviewed state for stories. One row per (story, viewer);
-- recording a view is idempotent (ON CONFLICT DO NOTHING in the repository).
CREATE TABLE story_views (
    story_id  uuid        NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    viewer_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    viewed_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (story_id, viewer_id)
);

-- "Which stories has this viewer seen" lookups. (The PK already serves the
-- per-story membership check.)
CREATE INDEX story_views_viewer_id_idx ON story_views (viewer_id);

COMMIT;
