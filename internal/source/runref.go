package source

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// RunRef addresses a run with its source. TraceIDs is filled by join between
// its two phases so the log adapters can match lines by trace as well as by run.id.
type RunRef struct {
	Source   string
	BoardID  string
	RunID    string
	TraceIDs []string
}

var ErrInvalidRef = errors.New("invalid run reference")

var (
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	attemptPattern = regexp.MustCompile(`^attempt_[A-Za-z0-9_-]{1,128}$`)
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func NewSuperpipelineRef(boardID, runID string) (RunRef, error) {
	if !idPattern.MatchString(boardID) {
		return RunRef{}, fmt.Errorf("%w: board id %q", ErrInvalidRef, boardID)
	}
	if !idPattern.MatchString(runID) {
		return RunRef{}, fmt.Errorf("%w: run id %q", ErrInvalidRef, runID)
	}
	return RunRef{Source: string(Superpipeline), BoardID: boardID, RunID: runID}, nil
}

// ParseRunRef parses "superpipeline:<board>/<run>".
func ParseRunRef(s string) (RunRef, error) {
	src, rest, ok := strings.Cut(s, ":")
	if !ok || src != string(Superpipeline) {
		return RunRef{}, fmt.Errorf("%w: want superpipeline:<board>/<run>, got %q", ErrInvalidRef, s)
	}
	board, run, ok := strings.Cut(rest, "/")
	if !ok {
		return RunRef{}, fmt.Errorf("%w: want superpipeline:<board>/<run>, got %q", ErrInvalidRef, s)
	}
	return NewSuperpipelineRef(board, run)
}

func (r RunRef) String() string { return r.Source + ":" + r.BoardID + "/" + r.RunID }

func (r RunRef) WithTraceIDs(ids []string) RunRef {
	r.TraceIDs = slices.Clone(ids)
	return r
}

func ValidAttemptID(id string) bool { return attemptPattern.MatchString(id) }
func ValidTraceID(id string) bool   { return traceIDPattern.MatchString(id) }
