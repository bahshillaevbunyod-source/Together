BEGIN;

-- gen_random_uuid() lives in pgcrypto on older PostgreSQL versions.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text,
    phone           text,
    username        text        NOT NULL,
    display_name    text        NOT NULL,
    avatar_url      text,
    bio             text,
    country_code    text,
    city            text,
    native_language text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    -- usernames are stored lowercase for case-insensitive identity
    CONSTRAINT users_username_lowercase CHECK (username = lower(username)),

    -- at least one contact method is required
    CONSTRAINT users_contact_present CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

-- username is always unique
CREATE UNIQUE INDEX users_username_key ON users (username);

-- email / phone are unique only when present (partial unique indexes)
CREATE UNIQUE INDEX users_email_key ON users (email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX users_phone_key ON users (phone) WHERE phone IS NOT NULL;

COMMIT;
