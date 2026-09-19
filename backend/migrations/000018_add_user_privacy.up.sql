BEGIN;

-- Account privacy flag. false = public account (current behavior for every
-- existing row); true = private account whose follows must be approved. Adding
-- it with a NOT NULL DEFAULT false is additive and preserves existing behavior.
ALTER TABLE users
    ADD COLUMN is_private boolean NOT NULL DEFAULT false;

COMMIT;
