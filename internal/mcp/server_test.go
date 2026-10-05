package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/mcp"
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
		Spans: d, Logs: d, Attempts: d,
		Verdicts: &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
	}
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
	if strings.Join(names, ",") != "get_run,list_run_logs,list_run_spans,record_verdict" {
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
