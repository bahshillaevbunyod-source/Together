BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM story_likes LIMIT 1)
       OR EXISTS (SELECT 1 FROM story_replies LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back 000032: story interaction data exists';
    END IF;
END;
$$;

DROP INDEX IF EXISTS story_views_story_viewed_idx;
DROP TABLE IF EXISTS story_replies;
DROP TABLE IF EXISTS story_likes;

COMMIT;
