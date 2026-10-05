-- +goose Up
-- Browser sessions. Not append-only: rows are touched as they are used and deleted at sign-out
-- and when they expire. The cookie's value is never stored, only its sha256.
CREATE TABLE sessions (
    id_hash        bytea       PRIMARY KEY CHECK (octet_length(id_hash) = 32),
    principal      text        NOT NULL,
    principal_kind text        NOT NULL CHECK (principal_kind IN ('human', 'agent', 'service')),
    tenant         text        NOT NULL,
    email          text        NULL,
    created_at     timestamptz NOT NULL,
    last_seen_at   timestamptz NOT NULL,
    expires_at     timestamptz NOT NULL
);

CREATE INDEX sessions_expires_idx   ON sessions (expires_at);
CREATE INDEX sessions_last_seen_idx ON sessions (last_seen_at);

-- +goose Down
DROP TABLE sessions;
