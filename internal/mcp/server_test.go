package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/mcp"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func mcpServer(t *testing.T) *httptest.Server {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	store := verdicts.NewMemStore()
	ops := &api.Ops{
		Join: &join.Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
			Verdicts: store, Principals: d},
		Spans: d, Logs: d, Attempts: d, Transcripts: d,
		Verdicts: &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
	}
	rs := runs.NewMemStore(store)
	started := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	scope, name := "brd_01", "Press"
	if _, err := rs.Upsert(context.Background(), []runs.Run{
		{Source: "superpipeline", ExternalRef: "brd_01/run_01", ScopeID: &scope, ScopeName: &name, Status: "succeeded",
			SourceStatus: "done", StartedAt: &started, ReportedAt: started},
		{Source: "superpipeline", ExternalRef: "brd_01/run_02", Status: "running", SourceStatus: "in_progress", ReportedAt: started},
	}, started); err != nil {
		t.Fatal(err)
	}
	ops.Runs = rs
	mux := http.NewServeMux()
	mux.Handle("/mcp", auth.Middleware(auth.DevAuthenticator{})(mcp.NewHandler(ops, "test")))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func connect(t *testing.T, srv *httptest.Server, token string) (*sdk.ClientSession, error) {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "v0"}, nil)
	return client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}},
	}, nil)
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: content = %+v", name, res.Content)
	}
	return res.Content[0].(*sdk.TextContent).Text, res.IsError
}

var runArgs = map[string]any{"source": "superpipeline", "board_id": "brd_01", "run_id": "run_01"}

func TestToolsListed(t *testing.T) {
	cs, err := connect(t, mcpServer(t), "dev:prn_human01:human")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "get_run,get_transcript,list_run_logs,list_run_spans,list_runs,record_verdict" {
		t.Errorf("tools = %v", names)
	}
}

func TestReadTools(t *testing.T) {
	cs, err := connect(t, mcpServer(t), "dev:prn_human01:human")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	text, isErr := call(t, cs, "get_run", runArgs)
	if isErr || !strings.Contains(text, `"ref":"superpipeline:brd_01/run_01"`) {
		t.Errorf("get_run: %s", text)
	}
	text, isErr = call(t, cs, "list_run_spans", map[string]any{"board_id": "brd_01", "run_id": "run_01", "limit": 2})
	var sp api.SpanPage
	_ = json.Unmarshal([]byte(text), &sp)
	if isErr || len(sp.Spans) != 2 || sp.NextCursor == "" {
		t.Errorf("list_run_spans (source defaulted): %s", text)
	}
	text, isErr = call(t, cs, "list_run_logs", map[string]any{"source": "superpipeline", "board_id": "brd_01", "run_id": "run_01", "level": "error"})
	if isErr || !strings.Contains(text, "bridge heartbeat failed") {
		t.Errorf("list_run_logs: %s", text)
	}
	text, isErr = call(t, cs, "get_run", map[string]any{"source": "github", "board_id": "brd_01", "run_id": "run_01"})
	if !isErr || !strings.Contains(text, "invalid_source") {
		t.Errorf("bad source: %v %s", isErr, text)
	}
}

func TestRecordVerdictRoundTrip(t *testing.T) {
	srv := mcpServer(t)
	cs, err := connect(t, srv, "dev:prn_canary:service")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	args := map[string]any{"idempotency_key": "canary-1", "subject_kind": "run",
		"subject_ref": "superpipeline:brd_01/run_01", "standard": "stage:review", "value": map[string]any{"score": 1}, "comment": "canary"}
	text, isErr := call(t, cs, "record_verdict", args)
	var v verdicts.Verdict
	_ = json.Unmarshal([]byte(text), &v)
	if isErr || v.ID == "" || v.Judge != "prn_canary" || v.JudgeKind != verdicts.JudgeGrader {
		t.Fatalf("record_verdict: %s", text)
	}
	text, _ = call(t, cs, "record_verdict", args)
	var again verdicts.Verdict
	_ = json.Unmarshal([]byte(text), &again)
	if again.ID != v.ID {
		t.Errorf("retry returned %s, want %s", again.ID, v.ID)
	}
	doc, _ := call(t, cs, "get_run", runArgs)
	if !strings.Contains(doc, `"id":"`+v.ID+`"`) {
		t.Errorf("verdict missing from get_run: %s", doc)
	}

	agent, err := connect(t, srv, "dev:prn_agent01:agent")
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	args["idempotency_key"] = "self-1"
	text, isErr = call(t, agent, "record_verdict", args)
	if !isErr || !strings.Contains(text, "self_judgement") {
		t.Errorf("self-judgement over MCP: %v %s", isErr, text)
	}
}

func TestMCPRequiresAuth(t *testing.T) {
	if _, err := connect(t, mcpServer(t), ""); err == nil {
		t.Error("connected without a token")
	}
}

func TestListRunsTool(t *testing.T) {
	cs, err := connect(t, mcpServer(t), "dev:prn_agent01:agent")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	text, isErr := call(t, cs, "list_runs", map[string]any{"status": []string{"succeeded"}, "needs_verdict": true})
	var page struct { // runs.Run has no UnmarshalJSON, so read the fields the test needs
		Runs []struct {
			ExternalRef string `json:"external_ref"`
		} `json:"runs"`
		Counts map[string]int `json:"counts"`
	}
	_ = json.Unmarshal([]byte(text), &page)
	if isErr || len(page.Runs) != 1 || page.Runs[0].ExternalRef != "brd_01/run_01" || page.Counts["running"] != 0 {
		t.Errorf("list_runs: %s", text)
	}
	text, isErr = call(t, cs, "list_runs", map[string]any{"status": []string{"done"}})
	if !isErr || !strings.Contains(text, "invalid_status") {
		t.Errorf("bad status: %v %s", isErr, text)
	}
}

func TestGetTranscriptTool(t *testing.T) {
	srv := mcpServer(t)
	plain, err := connect(t, srv, "dev:prn_grader01:agent:evidence:read")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	run := map[string]any{"board_id": "brd_01", "run_id": "run_01"}
	if text, isErr := call(t, plain, "get_transcript", run); !isErr || !strings.Contains(text, "transcripts_forbidden") {
		t.Errorf("an evidence-only token: %v %s", isErr, text)
	}

	cs, err := connect(t, srv, "dev:prn_grader01:agent:transcripts:read")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	text, isErr := call(t, cs, "get_transcript", run)
	if isErr || !strings.HasPrefix(text, `{"attempt_id":"attempt_01",`) || !strings.Contains(text, "[redacted:anthropic-key]") {
		t.Errorf("whole attempt: %v %s", isErr, text)
	}
	text, isErr = call(t, cs, "get_transcript", map[string]any{"board_id": "brd_01", "run_id": "run_01", "attempt_id": "attempt_01",
		"seq_from": 3, "seq_to": 4})
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(text), &page)
	if isErr || len(page.Items) != 1 || page.Items[0]["id"] != "tc_01" {
		t.Errorf("3..4: %v %s", isErr, text)
	}
	if text, isErr := call(t, cs, "get_transcript", map[string]any{"board_id": "brd_01", "run_id": "run_01", "seq_to": 10}); !isErr ||
		!strings.Contains(text, "range_outside_attempt") {
		t.Errorf("past the attempt: %v %s", isErr, text)
	}
	if text, isErr := call(t, cs, "get_transcript", map[string]any{"board_id": "brd_01", "run_id": "run_01", "attempt_id": "attempt_09"}); !isErr ||
		!strings.Contains(text, "attempt_not_found") {
		t.Errorf("unknown attempt: %v %s", isErr, text)
	}
}
