package runs

import (
	"encoding/json"
	"time"
)

// VerdictSummary is a run's latest verdict that no other verdict supersedes.
type VerdictSummary struct {
	ID        string          `json:"id"`
	Judge     string          `json:"judge"`
	JudgeKind string          `json:"judge_kind"`
	Standard  string          `json:"standard"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
}
