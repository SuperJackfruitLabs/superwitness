package verdicts

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies every pending migration. It is safe to run at every start.
func Migrate(ctx context.Context, dsn string) error {
	return migrate(dsn, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// MigrateTo applies pending migrations up to and including version. Tests use it to stand a
// database at an earlier release's schema before upgrading it.
func MigrateTo(ctx context.Context, dsn string, version int64) error {
	return migrate(dsn, func(p *goose.Provider) error {
		_, err := p.UpTo(ctx, version)
		return err
	})
}

func migrate(dsn string, run func(*goose.Provider) error) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := run(p); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
