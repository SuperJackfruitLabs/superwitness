//go:build integration

package testutil

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Roles is a disposable Postgres provisioned with the two-role SQL documented in README
// ("Deploying"): superwitness_owner owns the database and runs migrations;
// superwitness_app is the runtime role and never owns a table.
type Roles struct {
	OwnerDSN, AppDSN string
}

// StartPostgresWithRoles runs README's provision SQL, verbatim, as the container superuser.
// The tables do not exist yet: run the migrations over OwnerDSN, then call GrantRuntime.
func StartPostgresWithRoles(t *testing.T) Roles {
	t.Helper()
	ctx := context.Background()
	super := StartPostgres(t)
	admin := withDB(t, super, "postgres")
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// StartPostgres made a database named superwitness; the provision SQL creates its own.
	exec(t, conn, "DROP DATABASE superwitness")
	for _, stmt := range ReadmeSQL(t, "provision") {
		exec(t, conn, stmt)
	}
	// Passwords (or peer/cert auth) are per deployment, so README's SQL leaves them out.
	exec(t, conn, "ALTER ROLE superwitness_owner PASSWORD 'owner'")
	exec(t, conn, "ALTER ROLE superwitness_app PASSWORD 'app'")
	return Roles{OwnerDSN: withUser(t, super, "superwitness_owner", "owner"), AppDSN: withUser(t, super, "superwitness_app", "app")}
}

// GrantRuntime runs README's runtime-grants SQL, verbatim, as the owner role.
func (r Roles) GrantRuntime(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, r.OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, stmt := range ReadmeSQL(t, "runtime-grants") {
		exec(t, conn, stmt)
	}
}

// ReadmeSQL returns the statements of the ```sql block that follows `<!-- sql:name -->` in README.md.
func ReadmeSQL(t *testing.T, name string) []string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "<!-- sql:"+name+" -->")
	if !ok {
		t.Fatalf("README.md has no <!-- sql:%s --> block", name)
	}
	_, rest, ok = strings.Cut(rest, "```sql\n")
	if !ok {
		t.Fatalf("README.md: no ```sql block after <!-- sql:%s -->", name)
	}
	block, _, _ := strings.Cut(rest, "```")
	var lines []string
	for _, l := range strings.Split(block, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			lines = append(lines, l)
		}
	}
	var stmts []string
	for _, s := range strings.Split(strings.Join(lines, "\n"), ";") {
		if s = strings.TrimSpace(s); s != "" {
			stmts = append(stmts, s)
		}
	}
	if len(stmts) == 0 {
		t.Fatalf("README.md: <!-- sql:%s --> block is empty", name)
	}
	return stmts
}

func exec(t *testing.T, conn *pgx.Conn, stmt string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

func withDB(t *testing.T, dsn, db string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + db
	return u.String()
}

func withUser(t *testing.T, dsn, user, password string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}
