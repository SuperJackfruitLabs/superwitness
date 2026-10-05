package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunSubcommands(t *testing.T) {
	env := func(string) string { return "" }
	if code := run([]string{"version"}, env); code != 0 {
		t.Errorf("version = %d", code)
	}
	if code := run([]string{"bogus"}, env); code != 2 {
		t.Errorf("unknown subcommand = %d", code)
	}
	if code := run([]string{"rubric-add", "-id", "press"}, env); code != 2 {
		t.Errorf("rubric-add with missing flags = %d", code)
	}
	if code := run([]string{"serve"}, env); code != 2 {
		t.Errorf("serve without config = %d", code)
	}
}

func TestVersionForms(t *testing.T) {
	old := version
	version = "v9.9.9-test"
	defer func() { version = old }()
	env := func(string) string { return "" }
	for _, arg := range []string{"version", "--version", "-version"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = w
		code := run([]string{arg}, env)
		os.Stdout = stdout
		w.Close()
		out, _ := io.ReadAll(r)
		if code != 0 || string(out) != "v9.9.9-test\n" {
			t.Errorf("%s = %d, %q", arg, code, out)
		}
	}
}

func TestMigrateNeedsADatabaseURL(t *testing.T) {
	var stderr strings.Builder
	if code := migrate(func(string) string { return "" }, &stderr); code != 2 {
		t.Errorf("migrate without a DSN = %d", code)
	}
	if !strings.Contains(stderr.String(), "SW_MIGRATE_DATABASE_URL") {
		t.Errorf("stderr = %q; want it to name SW_MIGRATE_DATABASE_URL", stderr.String())
	}
}

func TestMigrateFailsCleanlyWhenPostgresIsDown(t *testing.T) {
	const dsn = "postgres://owner:pw@127.0.0.1:1/superwitness?sslmode=disable&connect_timeout=1"
	var stderr strings.Builder
	code := migrate(func(k string) string {
		return map[string]string{"SW_MIGRATE_DATABASE_URL": dsn}[k]
	}, &stderr)
	if code != 1 {
		t.Errorf("migrate with Postgres down = %d", code)
	}
	if !strings.HasPrefix(stderr.String(), "migrate: ") || strings.Contains(stderr.String(), "pw") {
		t.Errorf("stderr = %q; want a migrate: error that never shows the password", stderr.String())
	}
}

func TestUsageNamesMigrate(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = w
	code := run([]string{"bogus"}, func(string) string { return "" })
	os.Stderr = stderr
	w.Close()
	out, _ := io.ReadAll(r)
	if code != 2 || !strings.Contains(string(out), "migrate") {
		t.Errorf("usage = %d %q", code, out)
	}
}
