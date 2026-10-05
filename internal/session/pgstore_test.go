//go:build integration

package session

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

// The runtime role, with README's grants only: it can manage sessions, and nothing it could
// not before. (Task 4 adds the Manager issue/resolve round trip to this test.)
func TestPGStoreAsRuntimeRole(t *testing.T) {
	ctx := context.Background()
	roles := testutil.StartPostgresWithRoles(t)
	if err := verdicts.Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatal(err)
	}
	roles.GrantRuntime(t)
	pool, err := pgxpool.New(ctx, roles.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	storeContract(t, &PGStore{Pool: pool})

	for _, stmt := range []string{
		`TRUNCATE sessions`,
		`ALTER TABLE sessions ADD COLUMN sneaky int`,
		`DELETE FROM runs`,
		`UPDATE verdicts SET comment = 'edited'`,
		`DELETE FROM verdicts`,
	} {
		_, err := pool.Exec(ctx, stmt)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%q as runtime role: err = %v; want SQLSTATE 42501", stmt, err)
		}
	}
}

func TestPGStoreNotReady(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &PGStore{Pool: pool, Ready: func() bool { return false }}
	if _, err := s.Get(context.Background(), make([]byte, 32)); !errors.Is(err, ErrUnavailable) {
		t.Errorf("before migration: %v", err)
	}
}
