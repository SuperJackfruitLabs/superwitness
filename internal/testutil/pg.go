//go:build integration

// Package testutil starts throwaway dependencies for integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const PostgresImage = "postgres:17-alpine"

// StartPostgres runs a disposable Postgres and returns its DSN.
func StartPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, PostgresImage,
		postgres.WithDatabase("superwitness"), postgres.WithUsername("sw"), postgres.WithPassword("sw"),
		postgres.BasicWaitStrategies())
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}
