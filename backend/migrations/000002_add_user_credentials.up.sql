BEGIN;

-- bcrypt hash; never stores plaintext. Nullable for accounts without a
-- password yet (e.g. future phone/OAuth sign-in).
ALTER TABLE users ADD COLUMN password_hash text;

COMMIT;
