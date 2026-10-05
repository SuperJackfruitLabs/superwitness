package verdicts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const cols = `id, idempotency_key, kind, subject_kind, subject_ref, judge, judge_kind, standard, value, comment, evidence_refs, supersedes, created_at`
const colsV = `v.id, v.idempotency_key, v.kind, v.subject_kind, v.subject_ref, v.judge, v.judge_kind, v.standard, v.value, v.comment, v.evidence_refs, v.supersedes, v.created_at`

type PGStore struct{ Pool *pgxpool.Pool }

func scanVerdict(row pgx.Row) (Verdict, error) {
	var v Verdict
	var sk, jk string
	var value, refs []byte
	if err := row.Scan(&v.ID, &v.IdempotencyKey, &v.Kind, &sk, &v.SubjectRef, &v.Judge, &jk, &v.Standard,
		&value, &v.Comment, &refs, &v.Supersedes, &v.CreatedAt); err != nil {
		return Verdict{}, err
	}
	v.SubjectKind, v.JudgeKind = SubjectKind(sk), JudgeKind(jk)
	v.Value, v.EvidenceRefs = value, refs
	v.CreatedAt = v.CreatedAt.UTC()
	return v, nil
}

func unavailable(err error) error { return fmt.Errorf("%w: %v", ErrUnavailable, err) }

func (s *PGStore) Insert(ctx context.Context, v Verdict) (Verdict, bool, error) {
	row := s.Pool.QueryRow(ctx, `INSERT INTO verdicts (`+cols+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING `+cols,
		v.ID, v.IdempotencyKey, v.Kind, string(v.SubjectKind), v.SubjectRef, v.Judge, string(v.JudgeKind), v.Standard,
		string(v.Value), v.Comment, string(v.EvidenceRefs), v.Supersedes, v.CreatedAt)
	got, err := scanVerdict(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := s.GetByKey(ctx, v.IdempotencyKey)
		return existing, false, err
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "verdicts_supersedes_key" {
			return Verdict{}, false, ErrAlreadySuperseded
		}
		return Verdict{}, false, unavailable(err)
	}
	return got, true, nil
}

func (s *PGStore) one(ctx context.Context, where string, arg any) (Verdict, error) {
	v, err := scanVerdict(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM verdicts WHERE `+where, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return Verdict{}, ErrNotFound
	}
	if err != nil {
		return Verdict{}, unavailable(err)
	}
	return v, nil
}

func (s *PGStore) GetByKey(ctx context.Context, key string) (Verdict, error) {
	return s.one(ctx, "idempotency_key = $1", key)
}

func (s *PGStore) Get(ctx context.Context, id string) (Verdict, error) {
	return s.one(ctx, "id = $1", id)
}

func (s *PGStore) HasSuccessor(ctx context.Context, id string) (bool, error) {
	var has bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM verdicts WHERE supersedes = $1)`, id).Scan(&has); err != nil {
		return false, unavailable(err)
	}
	return has, nil
}

func (s *PGStore) ListCurrent(ctx context.Context, subjects []SubjectKey) ([]Verdict, error) {
	runs, attempts, cases := []string{}, []string{}, []string{}
	for _, k := range subjects {
		switch k.Kind {
		case SubjectRun:
			runs = append(runs, k.Ref)
		case SubjectAttempt:
			attempts = append(attempts, k.Ref)
		case SubjectEvalCaseRun:
			cases = append(cases, k.Ref)
		}
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+colsV+` FROM verdicts v
		WHERE ((v.subject_kind = 'run' AND v.subject_ref = ANY($1))
		    OR (v.subject_kind = 'attempt' AND v.subject_ref = ANY($2))
		    OR (v.subject_kind = 'eval_case_run' AND v.subject_ref = ANY($3)))
		  AND NOT EXISTS (SELECT 1 FROM verdicts n WHERE n.supersedes = v.id)
		ORDER BY v.created_at, v.id`, runs, attempts, cases)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	out := []Verdict{}
	for rows.Next() {
		v, err := scanVerdict(rows)
		if err != nil {
			return nil, unavailable(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable(err)
	}
	return out, nil
}

func (s *PGStore) RubricExists(ctx context.Context, id string, version int) (bool, error) {
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM rubrics WHERE id = $1 AND version = $2)`, id, version).Scan(&ok); err != nil {
		return false, unavailable(err)
	}
	return ok, nil
}

func (s *PGStore) InsertRubric(ctx context.Context, r Rubric) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO rubrics (id, version, name, scale, body, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, r.ID, r.Version, r.Name, string(r.Scale), r.Body, r.CreatedBy, r.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrRubricExists
	}
	if err != nil {
		return unavailable(err)
	}
	return nil
}

func (s *PGStore) Ping(ctx context.Context) source.SourceStatus {
	if err := s.Pool.Ping(ctx); err != nil {
		return source.ClassifyErr(ctx, err)
	}
	return source.StatusOK
}
