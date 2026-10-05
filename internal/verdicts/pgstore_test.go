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
