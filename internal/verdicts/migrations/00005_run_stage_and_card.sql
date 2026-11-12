-- +goose Up
-- A source may say which of its stages a run belongs to and which work item it is for, so that
-- one stage's history can be listed. Both are optional and follow the run's newest report.
ALTER TABLE runs
    ADD COLUMN stage_key text NULL CHECK (char_length(stage_key) BETWEEN 1 AND 128),
    ADD COLUMN card_id   text NULL CHECK (char_length(card_id) BETWEEN 1 AND 128);

CREATE INDEX runs_stage_idx ON runs (stage_key, scope_id, sort_at DESC);

-- +goose Down
DROP INDEX runs_stage_idx;
ALTER TABLE runs
    DROP COLUMN stage_key,
    DROP COLUMN card_id;
