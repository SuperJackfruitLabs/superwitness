// Package verdicts is superwitness's only writer: an append-only store of
// judgements, each naming a versioned standard, with the judge's kind taken from the
// hub's principal record.
package verdicts

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type SubjectKind string

const (
	SubjectRun         SubjectKind = "run"
	SubjectAttempt     SubjectKind = "attempt"
	SubjectEvalCaseRun SubjectKind = "eval_case_run"
)

type JudgeKind string

const (
	JudgeHuman  JudgeKind = "human"
	JudgeAgent  JudgeKind = "agent"
	JudgeGrader JudgeKind = "grader"
	JudgeRule   JudgeKind = "rule"
)

type Verdict struct {
	ID             string          `json:"id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Kind           string          `json:"kind"`
	SubjectKind    SubjectKind     `json:"subject_kind"`
	SubjectRef     string          `json:"subject_ref"`
	Judge          string          `json:"judge"`
	JudgeKind      JudgeKind       `json:"judge_kind"`
	Standard       string          `json:"standard"`
	Value          json.RawMessage `json:"value"`
	Comment        string          `json:"comment"`
	EvidenceRefs   json.RawMessage `json:"evidence_refs"`
	Supersedes     *string         `json:"supersedes"`
	CreatedAt      time.Time       `json:"created_at"`
}

type SubjectKey struct {
	Kind SubjectKind
	Ref  string
}

type Rubric struct {
	ID        string
	Version   int
	Name      string
	Scale     json.RawMessage
	Body      string
	CreatedBy string
	CreatedAt time.Time
}

var (
	ErrNotFound          = errors.New("verdict not found")
	ErrAlreadySuperseded = errors.New("verdict already superseded")
	ErrRubricExists      = errors.New("rubric version already exists")
	ErrUnavailable       = errors.New("verdict store unavailable")
)

type Store interface {
	Insert(ctx context.Context, v Verdict) (Verdict, bool, error)
	GetByKey(ctx context.Context, key string) (Verdict, error)
	Get(ctx context.Context, id string) (Verdict, error)
	HasSuccessor(ctx context.Context, id string) (bool, error)
	ListCurrent(ctx context.Context, subjects []SubjectKey) ([]Verdict, error)
	RubricExists(ctx context.Context, id string, version int) (bool, error)
	InsertRubric(ctx context.Context, r Rubric) error
}
