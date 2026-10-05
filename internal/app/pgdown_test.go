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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
)

// Nothing listens on port 1, so every connection is refused at once.
const unreachableDSN = "postgres://sw:sw@127.0.0.1:1/superwitness?sslmode=disable&connect_timeout=1"

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// A verdict Postgres that is down at start must not stop superwitness serving.
// Documents render with sources.verdicts "unavailable"; writes get a retryable 503.
func TestServesWhilePostgresIsDownAtStart(t *testing.T) {
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": unreachableDSN}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Build with Postgres down: %v", err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()
	client := &http.Client{Transport: bearer{"dev:prn_human01:human"}}

	resp, err := client.Get(srv.URL + "/v1/runs/superpipeline/brd_01/run_01")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sources  map[string]string `json:"sources"`
		Verdicts []struct {
			Kind string `json:"kind"`
		} `json:"verdicts"`
	}
	err = json.NewDecoder(resp.Body).Decode(&doc)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("run document: %d %v", resp.StatusCode, err)
	}
	if doc.Sources["verdicts"] != "unavailable" || doc.Sources["superpipeline"] != "ok" {
		t.Errorf("sources = %v; want verdicts unavailable, superpipeline ok", doc.Sources)
	}
	if len(doc.Verdicts) == 0 || doc.Verdicts[0].Kind != "gate" {
		t.Errorf("verdicts = %+v; want the superpipeline gates still shown", doc.Verdicts)
	}

	body := `{"idempotency_key":"k1","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"approved"}}`
	resp, err = client.Post(srv.URL+"/v1/verdicts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var perr struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&perr)
	resp.Body.Close()
	if resp.StatusCode != 503 || perr.Error.Code != "store_unavailable" || !perr.Error.Retryable {
		t.Errorf("POST /v1/verdicts: %d %+v; want 503 store_unavailable retryable", resp.StatusCode, perr)
	}

	resp, err = http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var health struct {
		Sources map[string]string `json:"sources"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()
	if resp.StatusCode != 200 || health.Sources["verdicts"] != "unavailable" {
		t.Errorf("/health: %d %v; want 200 with verdicts unavailable", resp.StatusCode, health.Sources)
	}

	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "v0"}, nil).Connect(context.Background(),
		&sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: client}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	text := func(res *sdk.CallToolResult) string { return res.Content[0].(*sdk.TextContent).Text }
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "get_run",
		Arguments: map[string]any{"source": "superpipeline", "board_id": "brd_01", "run_id": "run_01"}})
	if err != nil || res.IsError || !strings.Contains(text(res), `"verdicts":"unavailable"`) {
		t.Errorf("MCP get_run: %v %+v", err, res)
	}
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "record_verdict",
		Arguments: map[string]any{"idempotency_key": "k2", "subject_kind": "run", "subject_ref": "superpipeline:brd_01/run_01",
			"standard": "stage:review", "value": map[string]any{"decision": "approved"}}})
	if err != nil || !res.IsError || !strings.Contains(text(res), `"code":"store_unavailable"`) ||
		!strings.Contains(text(res), `"retryable":true`) {
		t.Errorf("MCP record_verdict: %v %s", err, text(res))
	}
}
