//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// With split DSNs, Build migrates over the owner DSN (the runtime role cannot create tables, so
// a fresh database only reaches verdicts:ok that way) and serves verdicts over the runtime DSN.
func TestBuildMigratesAsOwnerAndServesAsRuntimeRole(t *testing.T) {
	roles := testutil.StartPostgresWithRoles(t)
	cfg, err := config.Load(envOf(map[string]string{"SW_FAKE_SOURCES": "1",
		"SW_DATABASE_URL": roles.AppDSN, "SW_MIGRATE_DATABASE_URL": roles.OwnerDSN}))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()
	client := &http.Client{Transport: bearer{"dev:prn_human01:human"}}

	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), `"verdicts":"ok"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health never reported verdicts ok: %s", b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	roles.GrantRuntime(t) // the tables exist now; README's order is migrate, then grant

	body := `{"idempotency_key":"k-roles","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"approved"}}`
	resp, err := client.Post(srv.URL+"/v1/verdicts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var posted struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&posted)
	resp.Body.Close()
	if resp.StatusCode != 201 || posted.ID == "" {
		t.Fatalf("post verdict as runtime role: %d %+v", resp.StatusCode, posted)
	}
	resp, err = client.Get(srv.URL + "/v1/runs/superpipeline/brd_01/run_01")
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(doc), posted.ID) || !strings.Contains(string(doc), `"verdicts":"ok"`) {
		t.Errorf("run document does not show %s read back: %d %s", posted.ID, resp.StatusCode, doc)
	}
}

func tableOwner(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var owner string
	if err := conn.QueryRow(ctx, `SELECT tableowner FROM pg_tables WHERE tablename = 'verdicts'`).Scan(&owner); err != nil {
		t.Fatalf("verdicts table: %v", err)
	}
	return owner
}

// `superwitness migrate` needs only SW_MIGRATE_DATABASE_URL, and falls back to SW_DATABASE_URL.
func TestMigrateNeedsOnlyTheMigrateDSN(t *testing.T) {
	roles := testutil.StartPostgresWithRoles(t)
	if err := Migrate(context.Background(), envOf(map[string]string{"SW_MIGRATE_DATABASE_URL": roles.OwnerDSN})); err != nil {
		t.Fatalf("migrate with only SW_MIGRATE_DATABASE_URL: %v", err)
	}
	if got := tableOwner(t, roles.OwnerDSN); got != "superwitness_owner" {
		t.Errorf("verdicts owner = %q; want superwitness_owner", got)
	}
	// With both set, the migrate DSN wins: the runtime role could not migrate at all.
	if err := Migrate(context.Background(), envOf(map[string]string{
		"SW_MIGRATE_DATABASE_URL": roles.OwnerDSN, "SW_DATABASE_URL": roles.AppDSN})); err != nil {
		t.Errorf("migrate with both set: %v", err)
	}
}

func TestMigrateFallsBackToDatabaseURL(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	if err := Migrate(context.Background(), envOf(map[string]string{"SW_DATABASE_URL": dsn})); err != nil {
		t.Fatalf("migrate with only SW_DATABASE_URL: %v", err)
	}
	if got := tableOwner(t, dsn); got != "sw" {
		t.Errorf("verdicts owner = %q; want sw", got)
	}
}
