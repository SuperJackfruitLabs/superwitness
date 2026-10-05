//go:build integration

package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/SuperJackfruitLabs/superwitness/internal/app"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

const (
	// Pin these to the versions deployed in production; these are the floors tested.
	vtImage   = "victoriametrics/victoria-traces:v0.12.0"
	vlImage   = "victoriametrics/victoria-logs:v1.53.0"
	publicURL = "https://superwitness.test"
	digest    = "sha256:c1a88701c4409ae0eafb5f51ec7dabbe5f18037c1256baa045ab8cabf7ed3426"
)

func startVictoria(t *testing.T, image, port string) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image, ExposedPorts: []string{port},
			WaitingFor: wait.ForHTTP("/health").WithPort(port).WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start %s: %v", image, err)
	}
	ep, err := ctr.PortEndpoint(ctx, port, "http")
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

type hubFake struct {
	srv  *httptest.Server
	priv ed25519.PrivateKey
}

func svcOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svc-token" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		next(w, r)
	}
}

func newHub(t *testing.T) *hubFake {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pub := priv.Public().(ed25519.PublicKey)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "OKP", "crv": "Ed25519",
			"kid": "k1", "alg": "EdDSA", "x": base64.RawURLEncoding.EncodeToString(pub)}}})
	})
	mux.HandleFunc("POST /api/auth/service-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svc_sw:s3cret" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"token":"svc-token","expiresIn":300}`)
	})
	mux.HandleFunc("GET /api/evidence/runs/superpipeline/{run}", svcOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("run") != "run_01" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
			return
		}
		_, _ = w.Write(contracts.HubEvidenceRun)
	}))
	mux.HandleFunc("GET /api/evidence/attempts/{id}", svcOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "attempt_01" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
			return
		}
		_, _ = w.Write(contracts.HubEvidenceAttempt)
	}))
	mux.HandleFunc("GET /api/evidence/principals/{id}", svcOnly(func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "prn_human01":
			_, _ = w.Write(contracts.HubPrincipal)
		case "hubuser_7f3a", "prn_human02": // the hub accepts a hub auth user id as well as the prn_
			fmt.Fprint(w, `{"id":"prn_human02","kind":"human","handle":"former","suspended":false}`)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	h := &hubFake{srv: httptest.NewServer(mux), priv: priv}
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hubFake) token(t *testing.T, sub, kind string) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"iss": h.srv.URL, "aud": publicURL, "sub": sub,
		"principalKind": kind, "tenant": "fleet_01", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(h.priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newSuperpipeline(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/boards/{board}/runs/{run}/evidence", svcOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("board") != "brd_01" || r.PathValue("run") != "run_01" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"code":"RUN_NOT_FOUND"}}`)
			return
		}
		_, _ = w.Write(contracts.SuperpipelineEvidence)
	}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func tracerProvider(t *testing.T, vt, service string) *sdktrace.TracerProvider {
	t.Helper()
	exp, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(vt+"/insert/opentelemetry/v1/traces"))
	if err != nil {
		t.Fatal(err)
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))))
}

// seedTraces writes the hub's spans for run_01, plus a Workers span in a separate trace.
func seedTraces(t *testing.T, vt string) (string, context.Context) {
	t.Helper()
	hub := tracerProvider(t, vt, "agentpod-hub")
	tr := hub.Tracer("seed")
	ctx := context.Background()
	ctx1, dispatch := tr.Start(ctx, "dispatch", trace.WithAttributes(attribute.String("run.id", "run_01"),
		attribute.String("board.id", "brd_01"), attribute.String("card.id", "crd_01"), attribute.String("external.source", "superpipeline")))
	ctx2, attempt := tr.Start(ctx1, "attempt", trace.WithAttributes(attribute.String("attempt.id", "attempt_01"),
		attribute.String("station.id", "stn_01"), attribute.String("fingerprint.digest", digest), attribute.String("harness.name", "hermes"),
		attribute.String("acp.session_id", "acps_01"), attribute.Int64("acp.seq_from", 1), attribute.String("run.id", "run_01")))
	_, turn := tr.Start(ctx2, "turn", trace.WithAttributes(attribute.String("attempt.id", "attempt_01"),
		attribute.Int64("acp.seq_from", 1), attribute.Int64("acp.seq_to", 9)))
	turn.End()
	attempt.End()
	dispatch.End()
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	workers := tracerProvider(t, vt, "superpipeline-api")
	_, hb := workers.Tracer("seed").Start(ctx, "POST /v1/boards/:id/runs/:runId/heartbeat",
		trace.WithAttributes(attribute.String("run.id", "run_01")))
	hb.End()
	if err := workers.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	return dispatch.SpanContext().TraceID().String(), ctx2
}

// seedLogs writes an error line that carries only the attempt's trace context, and an info
// line that carries only run.id, so both arms of the run filter are exercised.
func seedLogs(t *testing.T, vl string, attemptCtx context.Context) {
	t.Helper()
	ctx := context.Background()
	exp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(vl+"/insert/opentelemetry/v1/logs"))
	if err != nil {
		t.Fatal(err)
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)),
		sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", "agentpod-hub"))))
	lg := lp.Logger("seed")
	var e otellog.Record
	e.SetTimestamp(time.Now())
	e.SetSeverity(otellog.SeverityError)
	e.SetSeverityText("ERROR")
	e.SetBody(attribute.StringValue("bridge heartbeat failed: 502 from superpipeline"))
	lg.Emit(attemptCtx, e)
	var i otellog.Record
	i.SetTimestamp(time.Now())
	i.SetSeverity(otellog.SeverityInfo)
	i.SetSeverityText("INFO")
	i.SetBody(attribute.StringValue("attempt opened"))
	i.AddAttributes(attribute.String("run.id", "run_01"))
	lg.Emit(ctx, i)
	if err := lp.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func call(t *testing.T, srv *httptest.Server, method, path, tok, body string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

func eventually(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("condition not met within %v", within)
}

func TestRunDocumentEndToEnd(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.StartPostgres(t)
	vt := startVictoria(t, vtImage, "10428/tcp")
	vl := startVictoria(t, vlImage, "9428/tcp")
	hub := newHub(t)
	sp := newSuperpipeline(t)
	hubTrace, attemptCtx := seedTraces(t, vt)
	seedLogs(t, vl, attemptCtx)

	secret := filepath.Join(t.TempDir(), "hub-secret")
	if err := os.WriteFile(secret, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := app.Build(ctx, config.Config{Listen: "127.0.0.1:0", PublicURL: publicURL, DatabaseURL: dsn,
		SuperpipelineURL: sp.URL, HubURL: hub.srv.URL, HubClientID: "svc_sw", HubClientSecretFile: secret,
		TracesURL: vt, LogsURL: vl, SourceTimeout: 5 * time.Second}, "it", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()
	human := hub.token(t, "prn_human01", "human")

	var doc join.RunDocument
	eventually(t, 120*time.Second, func() bool {
		code, b, _ := call(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_01", human, "")
		doc = join.RunDocument{}
		return code == 200 && json.Unmarshal(b, &doc) == nil && doc.Trace.Status == join.TracePartial &&
			len(doc.Errors) > 0 && doc.LogCount.Known && doc.LogCount.N >= 2 &&
			doc.Sources["verdicts"] == "ok" // migrations run in the background
	})

	for _, name := range []string{"superpipeline", "agentpod", "traces", "logs", "errors", "verdicts"} {
		if doc.Sources[name] != "ok" {
			t.Errorf("sources.%s = %s", name, doc.Sources[name])
		}
	}
	if len(doc.Trace.TraceIDs) != 2 || doc.Trace.TraceIDs[0] != hubTrace {
		t.Errorf("trace ids = %v, hub trace %s", doc.Trace.TraceIDs, hubTrace)
	}
	if len(doc.Attempts) != 1 || doc.Attempts[0].SpanCount != join.KnownCount(2) || doc.Attempts[0].Fingerprint.Digest != digest {
		t.Errorf("attempts = %+v", doc.Attempts)
	}
	if e := doc.Errors[0]; e.Message != "bridge heartbeat failed: 502 from superpipeline" || e.Service != "agentpod-hub" || e.TraceID != hubTrace {
		t.Errorf("error = %+v", e)
	}
	if doc.Run.Agent != "prn_agent01" {
		t.Errorf("run.agent = %s", doc.Run.Agent)
	}
	var g1, g2 join.VerdictView
	for _, v := range doc.Verdicts {
		switch v.ID {
		case "gate_01":
			g1 = v
		case "gate_02":
			g2 = v
		}
	}
	if g1.Judge != "prn_human01" || g1.JudgeKind != "human" {
		t.Errorf("gate_01 (decided_by_principal_id, kind from the hub principals route) = %+v", g1)
	}
	if g2.Judge != "prn_human02" || g2.JudgeKind != "human" {
		t.Errorf("gate_02 (resolved from decided_by_hub_sub) = %+v", g2)
	}

	// spans, paged
	code, b, _ := call(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_01/spans?limit=2", human, "")
	var p1 struct {
		Spans      []json.RawMessage `json:"spans"`
		NextCursor string            `json:"next_cursor"`
	}
	_ = json.Unmarshal(b, &p1)
	if code != 200 || len(p1.Spans) != 2 || p1.NextCursor == "" {
		t.Errorf("spans page 1: %d %s", code, b)
	}
	// logs, by level; the error line matched only by trace id
	code, b, _ = call(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_01/logs?level=error", human, "")
	if code != 200 || !strings.Contains(string(b), hubTrace) || !strings.Contains(string(b), `"trace_join":"ok"`) {
		t.Errorf("error logs: %d %s", code, b)
	}
	// by-attempt
	if code, _, h := call(t, srv, "GET", "/v1/runs/by-attempt/attempt_01", human, ""); code != 302 ||
		h.Get("Location") != "/v1/runs/superpipeline/brd_01/run_01" {
		t.Errorf("by-attempt: %d %v", code, h)
	}
	// verdicts: a service grader records; the executing agent is refused
	body := `{"idempotency_key":"it-1","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"score":1}}`
	code, b, _ = call(t, srv, "POST", "/v1/verdicts", hub.token(t, "prn_canary", "service"), body)
	var v struct{ ID, JudgeKind string }
	_ = json.Unmarshal(b, &v)
	if code != 201 || !strings.Contains(string(b), `"judge_kind":"grader"`) {
		t.Errorf("grader verdict: %d %s", code, b)
	}
	code, b, _ = call(t, srv, "POST", "/v1/verdicts", hub.token(t, "prn_agent01", "agent"),
		strings.Replace(body, "it-1", "it-2", 1))
	if code != 403 || !strings.Contains(string(b), "self_judgement") {
		t.Errorf("self-judgement: %d %s", code, b)
	}
	_, b, _ = call(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_01", human, "")
	if !strings.Contains(string(b), `"source":"superwitness"`) {
		t.Errorf("recorded verdict missing from the document: %s", b)
	}
	// a token minted for another audience is refused
	other := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"iss": hub.srv.URL, "aud": "https://app.superpipeline.dev",
		"sub": "prn_human01", "principalKind": "human", "tenant": "fleet_01", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix()})
	other.Header["kid"] = "k1"
	otherTok, _ := other.SignedString(hub.priv)
	if code, _, _ := call(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_01", otherTok, ""); code != 401 {
		t.Errorf("wrong audience: %d", code)
	}
}
