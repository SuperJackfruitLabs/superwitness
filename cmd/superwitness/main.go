package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/app"
	"github.com/SuperJackfruitLabs/superwitness/internal/canary"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/telemetry"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Getenv)) }

func run(args []string, getenv func(string) string) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(getenv)
	case "migrate":
		return migrate(getenv, os.Stderr)
	case "rubric-add":
		return rubricAdd(args, getenv, os.Stderr)
	case "canary":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return canary.Main(ctx, args, getenv, os.Stdout, os.Stderr)
	case "version", "--version", "-version":
		fmt.Println(version)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: superwitness [serve | migrate | rubric-add | canary | version | --version]")
		return 2
	}
}

func serve(getenv func(string) string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "superwitness:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTel, err := telemetry.Setup(ctx, cfg.OTLPEndpoint, version)
	if err != nil {
		logger.Error("telemetry", "err", err)
		return 1
	}
	a, err := app.Build(ctx, cfg, version, logger)
	if err != nil {
		logger.Error("startup", "err", err)
		return 1
	}
	defer a.Close()

	srv := &http.Server{Addr: cfg.Listen, Handler: a.Handler, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("superwitness listening", "addr", cfg.Listen, "version", version, "fake_sources", cfg.FakeSources)

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "err", err)
			return 1
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		logger.Error("http shutdown", "err", err)
	}
	if err := shutdownTel(sctx); err != nil {
		logger.Error("telemetry shutdown", "err", err)
	}
	return 0
}

// migrate runs the verdict migrations once over SW_MIGRATE_DATABASE_URL (else SW_DATABASE_URL)
// and exits. It needs no other setting, so an operator can run it as the owner role before serve.
func migrate(getenv func(string) string, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Migrate(ctx, getenv); err != nil {
		fmt.Fprintln(stderr, "migrate:", err)
		if errors.Is(err, config.ErrNoMigrateDSN) {
			return 2
		}
		return 1
	}
	fmt.Fprintln(stderr, "migrate: verdict schema is up to date")
	return 0
}

// rubricAdd records a rubric version. Rubrics are append-only, so a change is a new version.
func rubricAdd(args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("rubric-add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "rubric id, e.g. press")
	ver := fs.Int("version", 0, "rubric version, 1 or more")
	name := fs.String("name", "", "human-readable name")
	scale := fs.String("scale", "", `scale as JSON, e.g. {"min":0,"max":1}`)
	bodyFile := fs.String("body-file", "", "file holding the rubric text")
	createdBy := fs.String("created-by", "", "principal id of the author (prn_…)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *ver < 1 || *name == "" || *scale == "" || *bodyFile == "" || *createdBy == "" || !json.Valid([]byte(*scale)) {
		fmt.Fprintln(stderr, "rubric-add: -id, -version (>=1), -name, -scale (JSON), -body-file and -created-by are required")
		return 2
	}
	dsn := getenv("SW_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(stderr, "rubric-add: SW_DATABASE_URL is required")
		return 2
	}
	body, err := os.ReadFile(*bodyFile)
	if err != nil {
		fmt.Fprintln(stderr, "rubric-add:", err)
		return 1
	}
	ctx := context.Background()
	// Migrations run as the owner role when SW_MIGRATE_DATABASE_URL is set; the insert runs as the runtime role.
	if err := app.Migrate(ctx, getenv); err != nil {
		fmt.Fprintln(stderr, "rubric-add:", err)
		return 1
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(stderr, "rubric-add:", err)
		return 1
	}
	defer pool.Close()
	err = (&verdicts.PGStore{Pool: pool}).InsertRubric(ctx, verdicts.Rubric{ID: *id, Version: *ver, Name: *name,
		Scale: json.RawMessage(*scale), Body: string(body), CreatedBy: *createdBy, CreatedAt: time.Now().UTC()})
	if err != nil {
		fmt.Fprintln(stderr, "rubric-add:", err)
		return 1
	}
	fmt.Printf("rubric:%s@%d recorded\n", *id, *ver)
	return 0
}
