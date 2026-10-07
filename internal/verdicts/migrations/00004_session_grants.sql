-- +goose Up
-- Under the organization plane a session holds the plane's refresh token, sealed with a key
-- derived from the session cookie (which is never stored), so a copy of this table cannot use it.
ALTER TABLE sessions
    ADD COLUMN refresh_sealed    bytea       NULL,
    ADD COLUMN access_expires_at timestamptz NULL,
    ADD COLUMN unreachable_since timestamptz NULL;

-- +goose Down
ALTER TABLE sessions
    DROP COLUMN refresh_sealed,
    DROP COLUMN access_expires_at,
    DROP COLUMN unreachable_since;
