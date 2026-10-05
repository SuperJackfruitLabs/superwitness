-- +goose Up
-- The run registry: one row per run a source has reported. Unlike verdicts it is updatable,
-- but only forward: the application applies a report only when its reported_at is newer.
-- source and external_ref compare byte by byte (COLLATE "C"), as Go compares strings, so the
-- list's tie-break and its cursor agree in both.
CREATE TABLE runs (
    source        text COLLATE "C" NOT NULL CHECK (source ~ '^[a-z][a-z0-9-]{0,31}$'),
    external_ref  text COLLATE "C" NOT NULL CHECK (char_length(external_ref) BETWEEN 1 AND 256),
    scope_id      text        NULL CHECK (char_length(scope_id) BETWEEN 1 AND 128),
    scope_name    text        NULL CHECK (char_length(scope_name) BETWEEN 1 AND 200),
    title         text        NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    executor      text        NULL CHECK (char_length(executor) BETWEEN 1 AND 128),
    executor_name text        NULL CHECK (char_length(executor_name) BETWEEN 1 AND 200),
    status        text        NOT NULL CHECK (status IN ('queued', 'running', 'waiting', 'succeeded', 'failed', 'cancelled')),
    source_status text        NOT NULL CHECK (char_length(source_status) BETWEEN 1 AND 64),
    started_at    timestamptz NULL,
    ended_at      timestamptz NULL,
    reported_at   timestamptz NOT NULL,
    first_seen_at timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    -- The list's sort key: when the run started, or when superwitness first heard of it.
    sort_at       timestamptz GENERATED ALWAYS AS (coalesce(started_at, first_seen_at)) STORED,
    PRIMARY KEY (source, external_ref)
);

CREATE INDEX runs_sort_idx     ON runs (sort_at DESC, source, external_ref);
CREATE INDEX runs_scope_idx    ON runs (scope_id, sort_at DESC);
CREATE INDEX runs_status_idx   ON runs (status, sort_at DESC);
CREATE INDEX runs_executor_idx ON runs (executor, sort_at DESC);
CREATE INDEX runs_updated_idx  ON runs (updated_at DESC);

-- +goose Down
DROP TABLE runs;
