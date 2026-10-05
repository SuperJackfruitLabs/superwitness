package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore keeps the registry in the runs table, beside verdicts, which it reads for
// latest_verdict and needs_verdict. Ready gates it until the schema is migrated.
type PGStore struct {
	Pool  *pgxpool.Pool
	Ready func() bool
}

const upsertSQL = `INSERT INTO runs AS r (source, external_ref, scope_id, scope_name, title, executor, executor_name,
	status, source_status, started_at, ended_at, reported_at, first_seen_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
ON CONFLICT (source, external_ref) DO UPDATE SET
	scope_id = EXCLUDED.scope_id, scope_name = EXCLUDED.scope_name, title = EXCLUDED.title,
	executor = EXCLUDED.executor, executor_name = EXCLUDED.executor_name, status = EXCLUDED.status,
	source_status = EXCLUDED.source_status, started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at,
	reported_at = EXCLUDED.reported_at, updated_at = EXCLUDED.updated_at
WHERE r.reported_at < EXCLUDED.reported_at
RETURNING 1`

const runCols = `r.source, r.external_ref, r.scope_id, r.scope_name, r.title, r.executor, r.executor_name,
	r.status, r.source_status, r.started_at, r.ended_at, r.reported_at, r.first_seen_at, r.updated_at`

// The newest verdict on the run that no other verdict supersedes.
const latestJoin = `LEFT JOIN LATERAL (
	SELECT v.id, v.judge, v.judge_kind, v.standard, v.value, v.created_at FROM verdicts v
	WHERE v.subject_kind = 'run' AND v.subject_ref = r.source || ':' || r.external_ref
	  AND NOT EXISTS (SELECT 1 FROM verdicts n WHERE n.supersedes = v.id)
	ORDER BY v.created_at DESC, v.id DESC LIMIT 1) lv ON true`

const needsVerdictSQL = `r.status IN ('succeeded', 'failed', 'cancelled') AND NOT EXISTS (
	SELECT 1 FROM verdicts v WHERE v.subject_kind = 'run' AND v.subject_ref = r.source || ':' || r.external_ref)`

func unavailable(err error) error { return fmt.Errorf("%w: %v", ErrUnavailable, err) }

// compact drops the spaces jsonb puts in its text form.
func compact(raw []byte) json.RawMessage {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	return b.Bytes()
}

func (s *PGStore) ready() error {
	if s.Ready != nil && !s.Ready() {
		return ErrUnavailable
	}
	return nil
}

func (s *PGStore) Upsert(ctx context.Context, batch []Run, now time.Time) ([]bool, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	now = now.UTC().Truncate(time.Microsecond)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, unavailable(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit
	applied := make([]bool, len(batch))
	for i, r := range batch {
		var one int
		err := tx.QueryRow(ctx, upsertSQL, r.Source, r.ExternalRef, r.ScopeID, r.ScopeName, r.Title, r.Executor,
			r.ExecutorName, r.Status, r.SourceStatus, r.StartedAt, r.EndedAt, r.ReportedAt, now).Scan(&one)
		switch {
		case err == nil:
			applied[i] = true
		case errors.Is(err, pgx.ErrNoRows): // the stored report is as new or newer
		default:
			return nil, unavailable(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, unavailable(err)
	}
	return applied, nil
}

func clause(where []string) string {
	if len(where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(where, " AND ")
}

func (s *PGStore) List(ctx context.Context, f Filter) (Page, error) {
	if err := s.ready(); err != nil {
		return Page{}, err
	}
	var where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if f.Source != "" {
		where = append(where, "r.source = "+arg(f.Source))
	}
	if f.ScopeID != "" {
		where = append(where, "r.scope_id = "+arg(f.ScopeID))
	}
	if f.Executor != "" {
		where = append(where, "r.executor = "+arg(f.Executor))
	}
	if f.Since != nil {
		where = append(where, "r.sort_at >= "+arg(*f.Since))
	}
	if f.Until != nil {
		where = append(where, "r.sort_at < "+arg(*f.Until))
	}
	if f.NeedsVerdict {
		where = append(where, needsVerdictSQL)
	}

	page := Page{Runs: []Run{}, Counts: zeroCounts()}
	rows, err := s.Pool.Query(ctx, `SELECT r.status, count(*) FROM runs r`+clause(where)+` GROUP BY r.status`, args...)
	if err != nil {
		return Page{}, unavailable(err)
	}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return Page{}, unavailable(err)
		}
		page.Counts[st] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Page{}, unavailable(err)
	}

	if len(f.Statuses) > 0 {
		where = append(where, "r.status = ANY("+arg(f.Statuses)+")")
	}
	if c := f.After; c != nil {
		at, src, ref := arg(c.SortAt), arg(c.Source), arg(c.ExternalRef)
		where = append(where, fmt.Sprintf("(r.sort_at < %s OR (r.sort_at = %s AND (r.source, r.external_ref) > (%s, %s)))", at, at, src, ref))
	}
	limit := arg(f.Limit + 1)
	rows, err = s.Pool.Query(ctx, `SELECT `+runCols+`, lv.id, lv.judge, lv.judge_kind, lv.standard, lv.value, lv.created_at
		FROM runs r `+latestJoin+clause(where)+` ORDER BY r.sort_at DESC, r.source, r.external_ref LIMIT `+limit, args...)
	if err != nil {
		return Page{}, unavailable(err)
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return Page{}, unavailable(err)
		}
		page.Runs = append(page.Runs, r)
	}
	if err := rows.Err(); err != nil {
		return Page{}, unavailable(err)
	}
	if len(page.Runs) > f.Limit {
		page.Runs = page.Runs[:f.Limit]
		c := CursorOf(page.Runs[len(page.Runs)-1])
		page.Next = &c
	}
	return page, nil
}

func scanRun(rows pgx.Rows) (Run, error) {
	var r Run
	var vID, vJudge, vJudgeKind, vStandard *string
	var vValue []byte
	var vAt *time.Time
	if err := rows.Scan(&r.Source, &r.ExternalRef, &r.ScopeID, &r.ScopeName, &r.Title, &r.Executor, &r.ExecutorName,
		&r.Status, &r.SourceStatus, &r.StartedAt, &r.EndedAt, &r.ReportedAt, &r.FirstSeenAt, &r.UpdatedAt,
		&vID, &vJudge, &vJudgeKind, &vStandard, &vValue, &vAt); err != nil {
		return Run{}, err
	}
	utc := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		u := t.UTC()
		return &u
	}
	r.StartedAt, r.EndedAt = utc(r.StartedAt), utc(r.EndedAt)
	r.ReportedAt, r.FirstSeenAt, r.UpdatedAt = r.ReportedAt.UTC(), r.FirstSeenAt.UTC(), r.UpdatedAt.UTC()
	if vID != nil {
		r.LatestVerdict = &VerdictSummary{ID: *vID, Judge: *vJudge, JudgeKind: *vJudgeKind, Standard: *vStandard,
			Value: compact(vValue), CreatedAt: vAt.UTC()}
	}
	return r, nil
}
