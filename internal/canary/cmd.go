package canary

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const usage = `usage:
  superwitness canary run --trigger nightly|manual [--force-fail] --results PATH [--result-out PATH]
  superwitness canary record-alert --results PATH --ntfy-id ID --query TEXT
  superwitness canary status --results PATH [--json]`

const (
	callTimeout     = 30 * time.Second
	logsScanTimeout = 5 * time.Minute // a whole-window log scan streams a lot
	// tracesScanTimeout covers per-service window scans of up to SearchLimit traces each.
	tracesScanTimeout = 3 * time.Minute
)

// processDeadline bounds build + Run so the process ends, and still records and prints its
// result, before the systemd unit's 75-minute TimeoutStartSec. A var so tests can shorten it.
var processDeadline = 70 * time.Minute

type depsBuilder func(ctx context.Context, cfg Config, prior []Result) (Deps, error)

func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return runCmd(ctx, args[1:], getenv, stdout, stderr, NewDeps)
	case "record-alert":
		return recordAlertCmd(args[1:], stdout, stderr)
	case "status":
		return statusCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func runCmd(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, build depsBuilder) int {
	fs := flag.NewFlagSet("canary run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	trigger := fs.String("trigger", TriggerManual, "nightly or manual")
	force := fs.Bool("force-fail", false, "record a deliberate failure without dispatching (flow 10)")
	results := fs.String("results", "", "JSON-lines tracker to append to")
	out := fs.String("result-out", "", "also write this run's result here")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *trigger != TriggerNightly && *trigger != TriggerManual {
		fmt.Fprintln(stderr, "--trigger must be nightly or manual")
		return 2
	}
	if *results == "" {
		fmt.Fprintln(stderr, "--results is required")
		return 2
	}
	prior, err := LoadResults(*results)
	if err != nil {
		fmt.Fprintf(stderr, "canary: %v\n", err)
		return 2
	}

	var r Result
	switch {
	case *force:
		// No card, no clients: flow 10 is about the failure path, and a forced failure that
		// touched the live systems would be a real failure with a misleading label.
		now := time.Now().UTC()
		r = Result{V: ResultVersion, Trigger: *trigger, Forced: true, StartedAt: now,
			BoardID: strings.TrimSpace(getenv("SWC_BOARD_ID")),
			Error:   "forced failure: proving flow 10 (a canary failure reaches ntfy)"}
		r.Finish(now)
	default:
		cfg, err := LoadConfig(getenv)
		if err == nil {
			rctx, cancel := context.WithTimeout(ctx, processDeadline)
			var d Deps
			if d, err = build(rctx, cfg, prior); err == nil {
				r = Run(rctx, cfg, d, *trigger)
			}
			cancel()
		}
		if err != nil {
			now := time.Now().UTC()
			r = Result{V: ResultVersion, Trigger: *trigger, StartedAt: now, BoardID: cfg.BoardID, Error: "setup: " + err.Error()}
			r.Finish(now)
		}
	}

	if err := AppendResult(*results, r); err != nil {
		fmt.Fprintf(stderr, "canary: recording the result: %v\n", err)
		// An unrecorded run proves nothing and must not look like a pass.
		r.Pass = false
		r.Error += "; recording the result: " + err.Error()
		r.Error = strings.TrimPrefix(r.Error, "; ")
	}
	if *out != "" {
		b, _ := json.MarshalIndent(r, "", "  ")
		if err := os.WriteFile(*out, append(b, '\n'), 0o640); err != nil {
			fmt.Fprintf(stderr, "canary: writing %s: %v\n", *out, err)
		}
	}
	fmt.Fprintln(stdout, OneLine(r))
	if r.Pass {
		return 0
	}
	return 1
}

// recordAlertCmd is called by your alerting wrapper (anything that posts to ntfy and then calls
// `canary record-alert`), which alone holds the ntfy token, after ntfy has accepted AND stored the
// forced failure's message. It is flow 10's only proof.
func recordAlertCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("canary record-alert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	results := fs.String("results", "", "JSON-lines tracker")
	id := fs.String("ntfy-id", "", "the ntfy message id")
	query := fs.String("query", "", "how delivery was verified")
	if err := fs.Parse(args); err != nil || *results == "" || *id == "" || *query == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	rs, err := LoadResults(*results)
	if err != nil {
		fmt.Fprintf(stderr, "canary: %v\n", err)
		return 2
	}
	if len(rs) == 0 || !rs[len(rs)-1].Forced || rs[len(rs)-1].Pass {
		fmt.Fprintln(stderr, "canary: the last recorded run is not a forced failure; flow 10 not recorded")
		return 2
	}
	last := rs[len(rs)-1]
	now := time.Now().UTC()
	r := Result{V: ResultVersion, Trigger: TriggerAlert, StartedAt: now, EndedAt: now, BoardID: last.BoardID, Pass: true,
		Proofs: []FlowProof{{
			Flow:   10,
			RunRef: "forced:" + last.StartedAt.UTC().Format(time.RFC3339),
			Output: "ntfy accepted and stored message " + *id + " for the forced failure",
			Query:  *query,
			At:     now,
		}}}
	if err := AppendResult(*results, r); err != nil {
		fmt.Fprintf(stderr, "canary: recording flow 10: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "flow 10 recorded: ntfy message %s\n", *id)
	return 0
}

func statusCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("canary status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	results := fs.String("results", "", "JSON-lines tracker")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil || *results == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	rs, err := LoadResults(*results)
	if err != nil {
		fmt.Fprintf(stderr, "canary: %v\n", err)
		return 2
	}
	s := Summarize(rs)
	if *asJSON {
		b, _ := json.Marshal(s)
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	fmt.Fprint(stdout, FormatSummary(s))
	return 0
}

// NewDeps wires the production clients. It mints the service token up front, so a broken
// credential is a setup failure with its own message rather than nine failed checks.
func NewDeps(ctx context.Context, cfg Config, prior []Result) (Deps, error) {
	// Every client has a finite timeout: one hung call must not outlive the systemd unit's
	// 75-minute TimeoutStartSec. http.DefaultClient (no timeout) is never used.
	hc := &http.Client{Timeout: callTimeout}
	// Under the organization plane each audience has its own token: superpipeline's, and
	// superwitness's for /v1 and /mcp. Under the hub one token serves both.
	var spTokens, swTokens TokenSource
	issuer := "hub"
	if cfg.OrgPlaneURL != "" {
		issuer = "organization plane"
		spTokens = &PlaneServiceToken{PlaneURL: cfg.OrgPlaneURL, CredentialFile: cfg.OrgPlaneCredentialFile, Audience: cfg.SuperpipelineURL, HTTP: hc}
		swTokens = &PlaneServiceToken{PlaneURL: cfg.OrgPlaneURL, CredentialFile: cfg.OrgPlaneCredentialFile, Audience: cfg.SuperwitnessAudience, HTTP: hc}
	} else {
		hub := &ServiceToken{HubURL: cfg.HubURL, ClientID: cfg.ClientID, SecretFile: cfg.ClientSecretFile, HTTP: hc}
		spTokens, swTokens = hub, hub
	}
	tok, err := swTokens.Token(ctx)
	if err != nil {
		return Deps{}, fmt.Errorf("%s service token: %w", issuer, err)
	}
	principal, err := PrincipalFromJWT(tok)
	if err != nil {
		return Deps{}, err
	}
	return Deps{
		SP:          SuperpipelineClient{BaseURL: cfg.SuperpipelineURL, Tokens: spTokens, HTTP: hc},
		API:         SuperwitnessClient{BaseURL: cfg.SuperwitnessURL, Tokens: swTokens, HTTP: hc},
		MCP:         &MCPClient{URL: cfg.SuperwitnessURL + "/mcp", Tokens: swTokens, HTTP: hc},
		Traces:      TracesClient{BaseURL: cfg.TracesURL, TokenFile: cfg.TracesTokenFile, HTTP: &http.Client{Timeout: tracesScanTimeout}, SearchLimit: 1000},
		Logs:        LogsClient{BaseURL: cfg.LogsURL, TokenFile: cfg.LogsTokenFile, HTTP: &http.Client{Timeout: logsScanTimeout}},
		Errors:      OTLPLogs{URL: cfg.OTLPLogsURL, HTTP: hc},
		OTLPLogsURL: cfg.OTLPLogsURL,
		Principal:   principal,
		Now:         time.Now,
		Sleep:       sleepCtx,
		Rand:        rand.Reader,
		Prior:       prior,
	}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
