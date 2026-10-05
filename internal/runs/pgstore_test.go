//go:build integration

package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.StartPostgres(t)
	if err := verdicts.Migrate(context.Background(), dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPGStoreContract(t *testing.T) {
	pool := pgPool(t)
	vs := &verdicts.PGStore{Pool: pool}
	storeContract(t, &PGStore{Pool: pool}, func(v verdicts.Verdict) {
		if _, _, err := vs.Insert(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	})
}

// One bad row fails the whole batch: the first row is not left behind.
func TestPGStoreBatchIsOneTransaction(t *testing.T) {
	pool := pgPool(t)
	s := &PGStore{Pool: pool}
	bad := mk("brd_01/run_02", "bogus", 0, t0) // validation would refuse it; the CHECK refuses it here
	if _, err := s.Upsert(context.Background(), []Run{mk("brd_01/run_01", "running", 0, t0), bad}, t0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM runs`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows after a failed batch = %d %v; want 0", n, err)
	}
}

func TestPGStoreNotReady(t *testing.T) {
	s := &PGStore{Pool: pgPool(t), Ready: func() bool { return false }}
	if _, err := s.List(context.Background(), Filter{Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("list before migration: %v", err)
	}
}

// A deployment upgraded without README's new grant: the registry is unavailable, not broken.
func TestPGStoreWithoutGrantIsUnavailable(t *testing.T) {
	ctx := context.Background()
	roles := testutil.StartPostgresWithRoles(t)
	if err := verdicts.Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatal(err)
	}
	owner, err := pgxpool.New(ctx, roles.OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	// 0.0.1's grants only.
	for _, stmt := range []string{`GRANT USAGE ON SCHEMA public TO superwitness_app`,
		`GRANT SELECT, INSERT ON TABLE verdicts, rubrics TO superwitness_app`} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	app, err := pgxpool.New(ctx, roles.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	s := &PGStore{Pool: app}
	if _, err := s.Upsert(ctx, []Run{mk("brd_01/run_01", "running", 0, t0)}, t0); !errors.Is(err, ErrUnavailable) {
		t.Errorf("upsert without the grant: %v; want ErrUnavailable", err)
	}
	if _, err := s.List(ctx, Filter{Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("list without the grant: %v; want ErrUnavailable", err)
	}
}
