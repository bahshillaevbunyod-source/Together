BEGIN;

CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    description text CHECK (description IS NULL OR length(description) <= 10000),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz,
    timezone text NOT NULL DEFAULT 'UTC' CHECK (length(btrim(timezone)) BETWEEN 1 AND 100),
    event_type text NOT NULL CHECK (event_type IN ('in_person','online')),
    location_name text CHECK (location_name IS NULL OR length(location_name) <= 200),
    location_address text CHECK (location_address IS NULL OR length(location_address) <= 500),
    online_url text CHECK (online_url IS NULL OR length(online_url) <= 2048),
    visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public','followers','private')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT events_time_order CHECK (ends_at IS NULL OR ends_at >= starts_at),
    CONSTRAINT events_type_location CHECK (
        (event_type = 'in_person' AND online_url IS NULL)
        OR (event_type = 'online' AND location_name IS NULL AND location_address IS NULL)
    )
);

CREATE TABLE event_rsvps (
    event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('going','interested')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX events_upcoming_idx ON events (starts_at ASC, id ASC);
CREATE INDEX events_creator_idx ON events (creator_user_id, starts_at DESC, id DESC);
CREATE INDEX events_visibility_upcoming_idx ON events (visibility, starts_at ASC, id ASC);
CREATE INDEX event_rsvps_user_idx ON event_rsvps (user_id, updated_at DESC, event_id DESC);
CREATE INDEX event_rsvps_event_status_idx ON event_rsvps (event_id, status, updated_at DESC, user_id);

COMMIT;
