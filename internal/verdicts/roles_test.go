//go:build integration

package verdicts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

// The runtime role holds only README's runtime grants, so it can append and read but cannot
// rewrite history, and, not owning the tables, cannot switch the append-only triggers off.
func TestRuntimeRoleCanAppendButNotRewrite(t *testing.T) {
	ctx := context.Background()
	roles := testutil.StartPostgresWithRoles(t)
	if err := Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatalf("migrate as owner: %v", err)
	}
	roles.GrantRuntime(t)

	pool, err := pgxpool.New(ctx, roles.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &PGStore{Pool: pool}
	storeContract(t, s) // INSERT and SELECT on verdicts and rubrics, supersedes FK included

	for _, stmt := range []string{
		`UPDATE verdicts SET comment = 'edited'`,
		`DELETE FROM verdicts`,
		`TRUNCATE verdicts`,
		`UPDATE rubrics SET body = 'edited'`,
		`DELETE FROM rubrics`,
		`TRUNCATE rubrics`,
		`ALTER TABLE verdicts DISABLE TRIGGER verdicts_append_only`,
		`ALTER TABLE verdicts DISABLE TRIGGER ALL`,
		`ALTER TABLE rubrics DISABLE TRIGGER rubrics_append_only`,
		`DROP TRIGGER verdicts_append_only ON verdicts`,
		`DROP TRIGGER rubrics_no_truncate ON rubrics`,
		`DROP FUNCTION superwitness_refuse_mutation() CASCADE`,
		`DROP TABLE verdicts`,
		`CREATE TABLE sneaky (id int)`,
	} {
		// 42501 insufficient_privilege: refused for want of a grant, not by the triggers (P0001),
		// so a widened grant cannot pass here.
		_, err := pool.Exec(ctx, stmt)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%q as runtime role: err = %v; want SQLSTATE 42501", stmt, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verdicts`).Scan(&n); err != nil || n != 3 {
		t.Errorf("verdicts after refused writes = %d %v; want 3 untouched", n, err)
	}

	// The runtime role cannot migrate: goose's own table is the owner's alone.
	if err := Migrate(ctx, roles.AppDSN); err == nil {
		t.Error("runtime role ran the migrations")
	}
}

// When SW_MIGRATE_DATABASE_URL is kept out of the service env, serve's background migration
// runs as the runtime role. README says what that needs: SELECT on goose_db_version, after
// which the pass succeeds while nothing is pending and fails when a migration is pending.
func TestRuntimeRoleMigrationPass(t *testing.T) {
	ctx := context.Background()
	roles := testutil.StartPostgresWithRoles(t)
	if err := Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatalf("migrate as owner: %v", err)
	}
	roles.GrantRuntime(t)
	owner, err := pgxpool.New(ctx, roles.OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)

	if err := Migrate(ctx, roles.AppDSN); err == nil {
		t.Fatal("runtime role passed the migration check without SELECT on goose_db_version")
	}
	if _, err := owner.Exec(ctx, `GRANT SELECT ON goose_db_version TO superwitness_app`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, roles.AppDSN); err != nil {
		t.Errorf("runtime role, nothing pending: %v", err)
	}
	// Make the newest migration pending again (its table gone, its version row removed); the
	// runtime role cannot apply it, because that needs CREATE on the schema.
	if _, err := owner.Exec(ctx, `DROP TABLE sessions`); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id = 3`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, roles.AppDSN); err == nil {
		t.Error("runtime role applied a pending migration")
	}
	// The owner re-runs the pass: sessions comes back, and README's grants (re-applied here, as
	// an operator would after an upgrade) give the runtime role what it needs on it.
	if err := Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatalf("owner re-ran the pending migration: %v", err)
	}
	roles.GrantRuntime(t)
	var can bool
	for _, priv := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
		if err := owner.QueryRow(ctx, `SELECT has_table_privilege('superwitness_app', 'sessions', $1)`, priv).Scan(&can); err != nil || !can {
			t.Errorf("runtime role %s on sessions = %v %v; want granted", priv, can, err)
		}
	}
	for _, priv := range []string{"TRUNCATE", "REFERENCES", "TRIGGER"} {
		if err := owner.QueryRow(ctx, `SELECT has_table_privilege('superwitness_app', 'sessions', $1)`, priv).Scan(&can); err != nil || can {
			t.Errorf("runtime role %s on sessions = %v %v; want refused", priv, can, err)
		}
	}
	// The earlier tables' pass still holds at the new head: runs stays update-only.
	for priv, want := range map[string]bool{"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": false} {
		if err := owner.QueryRow(ctx, `SELECT has_table_privilege('superwitness_app', 'runs', $1)`, priv).Scan(&can); err != nil || can != want {
			t.Errorf("runtime role %s on runs = %v %v; want %v", priv, can, err, want)
		}
	}
}

// README's runtime grants let the running service add and update registry rows, never delete
// them, and leave verdicts as append-only as before.
func TestRuntimeRoleRegistryGrants(t *testing.T) {
	ctx := context.Background()
	roles := testutil.StartPostgresWithRoles(t)
	if err := Migrate(ctx, roles.OwnerDSN); err != nil {
		t.Fatalf("migrate as owner: %v", err)
	}
	roles.GrantRuntime(t)
	pool, err := pgxpool.New(ctx, roles.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `INSERT INTO runs (source, external_ref, status, source_status, reported_at, first_seen_at, updated_at)
		VALUES ('superpipeline', 'brd_01/run_01', 'running', 'in_progress', $1, $1, $1)`, now); err != nil {
		t.Fatalf("insert a run as the runtime role: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE runs SET status = 'succeeded', reported_at = $1 WHERE source = 'superpipeline'`,
		now.Add(time.Second)); err != nil {
		t.Fatalf("update a run as the runtime role: %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM runs`).Scan(&status); err != nil || status != "succeeded" {
		t.Errorf("status = %q %v", status, err)
	}
	for _, stmt := range []string{
		`DELETE FROM runs`,
		`TRUNCATE runs`,
		`ALTER TABLE runs ADD COLUMN sneaky int`,
		`DROP TABLE runs`,
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
