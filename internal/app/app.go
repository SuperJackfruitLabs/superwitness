// Package app wires config into a running handler: real adapters, or the development set.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/mcp"
	"github.com/SuperJackfruitLabs/superwitness/internal/ratelimit"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/session"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/agentpod"
	errsrc "github.com/SuperJackfruitLabs/superwitness/internal/source/errors"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/logs"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/superpipeline"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/traces"
	"github.com/SuperJackfruitLabs/superwitness/internal/telemetry"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
	"github.com/SuperJackfruitLabs/superwitness/internal/web"
)

type App struct {
	Handler http.Handler
	Close   func()
}

type wiring struct {
	sp, ap, tr, lg, er source.Source
	spans              source.SpanLister
	transcripts        source.TranscriptReader
	logLister          source.LogLister
	attempts           source.AttemptResolver
	principals         auth.PrincipalLookup
	authn              auth.Authenticator
	pingers            map[string]source.Pinger
}

// Migration retry backoff while the verdict Postgres is unreachable.
const (
	migrateMinWait = time.Second
	migrateMaxWait = 30 * time.Second
)

// MigrateTimeout bounds `superwitness migrate`, so it fails rather than hangs while Postgres is down.
const MigrateTimeout = 60 * time.Second

// Migrate runs the verdict migrations once over the effective migrate DSN
// (SW_MIGRATE_DATABASE_URL, else SW_DATABASE_URL). It reads no other setting.
func Migrate(ctx context.Context, getenv func(string) string) error {
	dsn, err := config.LoadMigrate(getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, MigrateTimeout)
	defer cancel()
	return verdicts.Migrate(ctx, dsn)
}

// Build wires the handler, ready to serve at once. The runtime pool uses SW_DATABASE_URL only;
// migrations run over cfg.MigrateDSN() (the owner role when SW_MIGRATE_DATABASE_URL is set).
// The verdict store opens in the background when its migrations succeed; until then it reports unavailable, so a
// Postgres that is down at start never takes run documents or get_run with it.
func Build(ctx context.Context, cfg config.Config, version string, logger *slog.Logger) (*App, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL) // parses the DSN; connects lazily
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	store := &verdicts.Gated{Inner: &verdicts.PGStore{Pool: pool}}
	hc := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	var w wiring
	if cfg.FakeSources {
		w, err = fakeWiring()
		logger.Warn("serving fake sources and accepting development tokens", "listen", cfg.Listen)
	} else {
		w, err = realWiring(cfg, hc)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	w.pingers["verdicts"] = store

	joiner := &join.Joiner{Superpipeline: w.sp, AgentPod: w.ap, Traces: w.tr, Logs: w.lg, Errors: w.er,
		Verdicts: store, Principals: w.principals, Timeout: cfg.SourceTimeout, Observe: telemetry.SourceObserver(logger)}
	subjects := &join.Subjects{Superpipeline: w.sp, AgentPod: w.ap, Attempts: w.attempts, Timeout: cfg.SourceTimeout}
	// Every bearer token names its caller by prn_ id, whatever the hub put in sub.
	authn := auth.Resolving{Inner: w.authn, Principals: w.principals}
	ops := &api.Ops{Join: joiner, Spans: w.spans, Logs: w.logLister, Attempts: w.attempts, Timeout: cfg.SourceTimeout,
		Verdicts:    &verdicts.Service{Store: store, Subjects: subjects},
		Runs:        &runs.PGStore{Pool: pool, Ready: store.Ready}, // same database, same migration gate
		RunSources:  cfg.RunSources,
		Rubrics:     store,
		History:     store,
		Transcripts: w.transcripts,
		Logger:      logger}
	// One trust list for the edge guard and for the rate limits' client IP.
	trusted := cfg.TrustedProxies
	if len(trusted) == 0 {
		if trusted, err = ratelimit.HostPrefixes(); err != nil {
			pool.Close()
			return nil, fmt.Errorf("SW_TRUSTED_PROXIES: reading this host's addresses: %w", err)
		}
	}
	// api.Server only sends non-API GETs to Web (never /v1/* or /mcp), so an unknown /v1/foo is the API's JSON 404.
	srv := &api.Server{Ops: ops, Auth: authn, Health: &api.Health{Version: version, Pingers: w.pingers},
		MCP: mcp.NewHandler(ops, version), Web: web.Handler(), Logger: logger, PublicURL: cfg.PublicURL,
		TrustedProxies: trusted}
	sessions, err := signIn(cfg, srv, w, store, pool, hc, trusted, logger)
	if err != nil {
		pool.Close()
		return nil, err
	}
	built, err := srv.Build()
	if err != nil {
		pool.Close()
		return nil, err
	}
	handler, err := instrument(built, cfg.PublicURL)
	if err != nil {
		pool.Close()
		return nil, err
	}

	mctx, cancel := context.WithCancel(ctx)
	migrated, swept := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(migrated)
		store.OpenWhenMigrated(mctx, func(c context.Context) error { return verdicts.Migrate(c, cfg.MigrateDSN()) },
			migrateMinWait, migrateMaxWait, logger)
	}()
	go func() {
		defer close(swept)
		if sessions != nil {
			sessions.Sweep(mctx, session.SweepEvery, logger)
		}
	}()
	return &App{Handler: handler, Close: func() { cancel(); <-migrated; <-swept; pool.Close() }}, nil
}

// signIn sets the rate limits, and turns on browser sign-in when SW_ALLOWED_PRINCIPALS names
// anyone. It returns the session manager, or nil when sign-in is off.
func signIn(cfg config.Config, srv *api.Server, w wiring, store *verdicts.Gated, pool *pgxpool.Pool,
	hc *http.Client, trusted []netip.Prefix, logger *slog.Logger) (*session.Manager, error) {
	srv.Limits = &api.Limits{
		Auth:          ratelimit.PerMinute(api.AuthPerMinute, api.AuthBurst),
		SessionWrites: ratelimit.PerMinute(api.SessionWritesPerMinute, api.SessionWriteBurst),
		Reports:       ratelimit.PerMinute(api.ReportsPerMinute, api.ReportBurst),
		ClientIP:      func(r *http.Request) string { return ratelimit.ClientIP(r, trusted) },
	}
	if !cfg.AppEnabled() {
		srv.Login = session.Off()
		return nil, nil
	}
	secret, err := auth.ReadSecretFile(cfg.SessionSecretFile)
	if err != nil {
		return nil, fmt.Errorf("SW_SESSION_SECRET_FILE: %w", err)
	}
	key, err := session.DeriveLoginKey([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("SW_SESSION_SECRET_FILE: %w", err)
	}
	allowed := map[string]bool{}
	for _, p := range cfg.AllowedPrincipals {
		allowed[p] = true
	}
	origin, err := auth.PublicOrigin(cfg.PublicURL)
	if err != nil {
		return nil, fmt.Errorf("sign-in needs SW_PUBLIC_URL's origin: %w", err)
	}
	m := &session.Manager{Store: &session.PGStore{Pool: pool, Ready: store.Ready}, Allowed: allowed}
	cookies := session.Cookies{Secure: strings.HasPrefix(cfg.PublicURL, "https://")}
	login := &session.Login{HubURL: cfg.HubURL, ClientID: cfg.AppClientID, PublicURL: cfg.PublicURL, Origin: origin,
		Key: key, Tokens: auth.NewVerifier(cfg.HubURL, cfg.PublicURL, hc), Principals: w.principals, Sessions: m,
		Cookies: cookies, HTTP: hc, Logger: logger}
	srv.Sessions, srv.SessionCookie, srv.Login = m, cookies.SessionName(), login.Handler()
	logger.Info("browser sign-in on", "allowed", len(allowed), "secure_cookies", cookies.Secure)
	return m, nil
}

// instrument wraps h in otelhttp with a fixed server name and port taken from SW_PUBLIC_URL.
// Without one, otelhttp labels http.server.* metrics with server.address and server.port from
// the request's Host header, which any unauthenticated client controls (unbounded cardinality).
func instrument(h http.Handler, publicURL string, opts ...otelhttp.Option) (http.Handler, error) {
	name, err := serverName(publicURL)
	if err != nil {
		return nil, err
	}
	return otelhttp.NewHandler(h, "superwitness", append([]otelhttp.Option{otelhttp.WithServerName(name)}, opts...)...), nil
}

// serverName is SW_PUBLIC_URL's host:port, with the scheme's default port made explicit so
// otelhttp never falls back to the Host header for the port.
func serverName(publicURL string) (string, error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("SW_PUBLIC_URL %q: want an absolute URL", publicURL)
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port), nil
}

func fakeWiring() (wiring, error) {
	d, err := fake.NewDev()
	if err != nil {
		return wiring{}, err
	}
	return wiring{sp: d.SP, ap: d.AP, tr: d.Traces, lg: d.Logs, er: d.Errors,
		spans: d, transcripts: d, logLister: d, attempts: d, principals: d, authn: auth.DevAuthenticator{},
		pingers: map[string]source.Pinger{"superpipeline": d, "agentpod": d, "traces": d, "logs": d}}, nil
}

func realWiring(cfg config.Config, hc *http.Client) (wiring, error) {
	secret, err := auth.ReadSecretFile(cfg.HubClientSecretFile)
	if err != nil {
		return wiring{}, fmt.Errorf("SW_HUB_CLIENT_SECRET_FILE: %w", err)
	}
	tracesBearer, err := optionalSecret(cfg.TracesTokenFile)
	if err != nil {
		return wiring{}, fmt.Errorf("SW_TRACES_TOKEN_FILE: %w", err)
	}
	logsBearer, err := optionalSecret(cfg.LogsTokenFile)
	if err != nil {
		return wiring{}, fmt.Errorf("SW_LOGS_TOKEN_FILE: %w", err)
	}
	tokens := auth.NewHubTokenSource(cfg.HubURL, cfg.HubClientID, secret, hc)
	sp := superpipeline.New(cfg.SuperpipelineURL, tokens, hc)
	ap := agentpod.New(cfg.HubURL, tokens, hc)
	tr := traces.New(cfg.TracesURL, tracesBearer, hc)
	lc := logs.NewClient(cfg.LogsURL, logsBearer, hc)
	lg := logs.New(lc)
	return wiring{sp: sp, ap: ap, tr: tr, lg: lg, er: errsrc.New(lc),
		spans: tr, transcripts: ap, logLister: lg, attempts: ap, principals: ap,
		authn:   auth.NewVerifier(cfg.HubURL, cfg.PublicURL, hc),
		pingers: map[string]source.Pinger{"superpipeline": sp, "agentpod": ap, "traces": tr, "logs": lg}}, nil
}

func optionalSecret(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return auth.ReadSecretFile(path)
}
