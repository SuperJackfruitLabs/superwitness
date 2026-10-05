//go:build integration

package verdicts

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

func pgStore(t *testing.T) *PGStore {
	t.Helper()
	dsn := testutil.StartPostgres(t)
	ctx := context.Background()
	if err := Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, dsn); err != nil { // idempotent
		t.Fatalf("second migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &PGStore{Pool: pool}
}

func TestPGStoreContract(t *testing.T) { storeContract(t, pgStore(t)) }

func TestPGStoreHistoryKeepsTheNewest(t *testing.T) { historyContract(t, pgStore(t)) }

func TestPGStoreIsAppendOnly(t *testing.T) {
	s := pgStore(t)
	ctx := context.Background()
	storeContract(t, s)
	for _, stmt := range []string{
		`UPDATE verdicts SET comment = 'edited'`,
		`DELETE FROM verdicts`,
		`TRUNCATE verdicts`,
		`UPDATE rubrics SET body = 'edited'`,
		`DELETE FROM rubrics`,
	} {
		_, err := s.Pool.Exec(ctx, stmt)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v", stmt, err)
		}
	}
}

// jsonb refuses a lone surrogate escape, which is why normalize must refuse it first.
func TestPGStoreRejectsLoneSurrogate(t *testing.T) {
	s := pgStore(t)
	v := verdict("vrd_s", "ks", nil, mustNow())
	v.Value = []byte(`{"text":"\ud800"}`)
	if _, _, err := s.Insert(context.Background(), v); err == nil {
		t.Error("jsonb accepted a lone surrogate; the 400 in normalize would then be unnecessary")
	}
	if !hasLoneSurrogate(v.Value) {
		t.Error("normalize's scan does not catch the value Postgres rejects")
	}
}

func TestPGStoreRejectsUnversionedStandard(t *testing.T) {
	s := pgStore(t)
	v := verdict("vrd_x", "kx", nil, mustNow())
	v.Standard = "rubric:press" // no version
	if _, _, err := s.Insert(context.Background(), v); err == nil {
		t.Error("database accepted a standard without a version")
	}
}

func TestPGStorePing(t *testing.T) {
	if st := pgStore(t).Ping(context.Background()); st != source.StatusOK {
		t.Errorf("ping = %s", st)
	}
}

// Postgres stores microseconds; truncating keeps round-trip comparisons equal.
func mustNow() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// A database at 0.0.1's schema, holding verdicts and a rubric, upgrades in place.
func TestUpgradeFromFirstRelease(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.StartPostgres(t)
	if err := MigrateTo(ctx, dsn, 1); err != nil {
		t.Fatalf("migrate to 0.0.1's schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var before bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.runs') IS NOT NULL`).Scan(&before); err != nil || before {
		t.Fatalf("runs exists at version 1: %v %v", before, err)
	}
	storeContract(t, &PGStore{Pool: pool}) // leaves verdicts and rubrics behind
	count := func() (verdictsN, rubricsN int) {
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM verdicts), (SELECT count(*) FROM rubrics)`).
			Scan(&verdictsN, &rubricsN); err != nil {
			t.Fatal(err)
		}
		return verdictsN, rubricsN
	}
	v0, r0 := count()

	if err := Migrate(ctx, dsn); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var after bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.runs') IS NOT NULL`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if v1, r1 := count(); v0 != 3 || v1 != v0 || r1 != r0 || r0 == 0 || !after {
		t.Errorf("verdicts %d → %d, rubrics %d → %d, runs table %v; want 3 verdicts and the rubrics kept, and the table", v0, v1, r0, r1, after)
	}
	// The generated sort key and the C collation are what the list query relies on.
	var gen, coll string
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT is_generated FROM information_schema.columns WHERE table_name = 'runs' AND column_name = 'sort_at'),
		(SELECT collation_name FROM information_schema.columns WHERE table_name = 'runs' AND column_name = 'external_ref')`).
		Scan(&gen, &coll); err != nil || gen != "ALWAYS" || coll != "C" {
		t.Errorf("sort_at generated = %q, external_ref collation = %q, %v", gen, coll, err)
	}
}
