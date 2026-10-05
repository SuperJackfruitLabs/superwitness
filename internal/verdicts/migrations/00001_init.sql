-- +goose Up
CREATE TABLE rubrics (
    id         text        NOT NULL,
    version    integer     NOT NULL CHECK (version >= 1),
    name       text        NOT NULL,
    scale      jsonb       NOT NULL,
    body       text        NOT NULL,
    created_by text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, version)
);

CREATE TABLE verdicts (
    id              text        PRIMARY KEY,
    idempotency_key text        NOT NULL UNIQUE,
    kind            text        NOT NULL CHECK (kind IN ('grader', 'review', 'eval', 'calibration')),
    subject_kind    text        NOT NULL CHECK (subject_kind IN ('run', 'attempt', 'eval_case_run')),
    subject_ref     text        NOT NULL,
    judge           text        NOT NULL,
    judge_kind      text        NOT NULL CHECK (judge_kind IN ('human', 'agent', 'grader', 'rule')),
    standard        text        NOT NULL CHECK (standard ~ '^(rubric:[^@[:space:]]+@[0-9]+|stage:[^[:space:]]+|case:[^[:space:]]+)$'),
    value           jsonb       NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    comment         text        NOT NULL DEFAULT '',
    evidence_refs   jsonb       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(evidence_refs) = 'array'),
    supersedes      text        NULL REFERENCES verdicts (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- a chain stays linear: each verdict is superseded at most once
    CONSTRAINT verdicts_supersedes_key UNIQUE (supersedes)
);

CREATE INDEX verdicts_subject_idx ON verdicts (subject_kind, subject_ref);

-- Append-only: refuse UPDATE, DELETE and TRUNCATE at the database itself.
-- +goose StatementBegin
CREATE FUNCTION superwitness_refuse_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'superwitness: % on % is refused; verdicts and rubrics are append-only', TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'P0001';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER verdicts_append_only BEFORE UPDATE OR DELETE ON verdicts
    FOR EACH ROW EXECUTE FUNCTION superwitness_refuse_mutation();
CREATE TRIGGER verdicts_no_truncate BEFORE TRUNCATE ON verdicts
    FOR EACH STATEMENT EXECUTE FUNCTION superwitness_refuse_mutation();
CREATE TRIGGER rubrics_append_only BEFORE UPDATE OR DELETE ON rubrics
    FOR EACH ROW EXECUTE FUNCTION superwitness_refuse_mutation();
CREATE TRIGGER rubrics_no_truncate BEFORE TRUNCATE ON rubrics
    FOR EACH STATEMENT EXECUTE FUNCTION superwitness_refuse_mutation();

-- +goose Down
DROP TABLE verdicts;
DROP TABLE rubrics;
DROP FUNCTION superwitness_refuse_mutation();
