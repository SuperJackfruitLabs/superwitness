package canary

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

func fakeJWT(sub string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(`{"sub":"`+sub+`"}`)) + ".sig"
}

func TestServiceTokenExchangesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/api/auth/service-token" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer svc_canary:s3cret" {
			t.Errorf("authorization = %q", got)
		}
		fmt.Fprintf(w, `{"token":"tok-%d","expiresIn":300}`, calls)
	}))
	defer srv.Close()
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte("s3cret\n"), 0o600)
	now := time.Date(2026, 10, 10, 2, 30, 0, 0, time.UTC)
	st := &ServiceToken{HubURL: srv.URL + "/", ClientID: "svc_canary", SecretFile: secret, Now: func() time.Time { return now }}
	ctx := context.Background()
	if tok, err := st.Token(ctx); err != nil || tok != "tok-1" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if tok, _ := st.Token(ctx); tok != "tok-1" || calls != 1 {
		t.Fatalf("cached token refetched: tok=%q calls=%d", tok, calls)
	}
	now = now.Add(4*time.Minute + 45*time.Second) // inside the token source's 30s refresh margin
	if tok, _ := st.Token(ctx); tok != "tok-2" || calls != 2 {
		t.Fatalf("near-expiry token not refreshed: tok=%q calls=%d", tok, calls)
	}
}

func TestServiceTokenErrorNeverCarriesTheSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte("s3cret"), 0o600)
	_, err := (&ServiceToken{HubURL: srv.URL, ClientID: "svc_canary", SecretFile: secret}).Token(context.Background())
	if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrincipalFromJWT(t *testing.T) {
	if p, err := PrincipalFromJWT(fakeJWT("prn_canary")); err != nil || p != "prn_canary" {
		t.Fatalf("p=%q err=%v", p, err)
	}
	if _, err := PrincipalFromJWT("not-a-jwt"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestSuperpipelineClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svc-token" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/boards/brd_c/cards":
			var body struct {
				Title string         `json:"title"`
				Spec  map[string]any `json:"spec"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Title == "refuse" {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"error":{"code":"BOARD_NOT_PERMITTED","message":"no"}}`)
				return
			}
			if body.Spec["task"] == nil {
				t.Errorf("spec not sent: %+v", body)
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"card":{"id":"crd_1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/boards/brd_c/cards/crd_1/attempts":
			fmt.Fprint(w, `{"attempts":[{"runId":"run_1","stageKey":"work","agentId":"agt_kai","status":"ended","outcome":"completed","startedAt":"x","endedAt":"y"}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := SuperpipelineClient{BaseURL: srv.URL, Tokens: staticToken("svc-token")}
	ctx := context.Background()
	id, err := c.CreateCard(ctx, "brd_c", "canary", map[string]any{"task": "x"})
	if err != nil || id != "crd_1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := c.CreateCard(ctx, "brd_c", "refuse", map[string]any{"task": "x"}); err == nil || !strings.Contains(err.Error(), "BOARD_NOT_PERMITTED") {
		t.Errorf("err = %v", err)
	}
	atts, err := c.Attempts(ctx, "brd_c", "crd_1")
	if err != nil || len(atts) != 1 || atts[0].RunID != "run_1" || *atts[0].Outcome != "completed" {
		t.Fatalf("atts=%+v err=%v", atts, err)
	}
}

func TestSuperwitnessGetRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/superpipeline/brd_c/run_1" || r.Header.Get("Authorization") != "Bearer svc-token" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(goodDoc())
	}))
	defer srv.Close()
	doc, err := SuperwitnessClient{BaseURL: srv.URL, Tokens: staticToken("svc-token")}.GetRun(context.Background(), "brd_c", "run_1")
	if err != nil || doc.Run.Agent != "prn_exec01" || len(doc.Attempts) != 1 {
		t.Fatalf("doc=%+v err=%v", doc, err)
	}
}

func TestSuperwitnessLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/superpipeline/brd_c/run_1/logs" || r.Header.Get("Authorization") != "Bearer svc-token" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"logs":[{"service":"agentpod-hub","level":"info","message":"m","trace_id":"t1","run_id":"run_1","at":"x"}],"next_cursor":null}`)
	}))
	defer srv.Close()
	lines, err := SuperwitnessClient{BaseURL: srv.URL, Tokens: staticToken("svc-token")}.Logs(context.Background(), "brd_c", "run_1")
	if err != nil || len(lines) != 1 || lines[0].RunID != "run_1" || lines[0].Service != "agentpod-hub" {
		t.Fatalf("lines=%+v err=%v", lines, err)
	}
}

func TestOTLPInjectError(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/logs" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	at := time.Date(2026, 10, 10, 2, 45, 0, 0, time.UTC)
	err := OTLPLogs{URL: srv.URL + "/v1/logs"}.InjectError(context.Background(), "run_1", "0af7651916cd43dd8448eb211c80319c", "swcerr-0000abcd", at)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"stringValue":"superwitness-canary"`, `"severityText":"ERROR"`, `"severityNumber":17`,
		`"traceId":"0af7651916cd43dd8448eb211c80319c"`, `"key":"run.id"`, `"stringValue":"run_1"`, "swcerr-0000abcd",
		`"timeUnixNano":"` + strconv.FormatInt(at.UnixNano(), 10) + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("payload lacks %s: %s", want, body)
		}
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer down.Close()
	if err := (OTLPLogs{URL: down.URL}).InjectError(context.Background(), "run_1", "", "n", at); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("err = %v", err)
	}
}

func fakeMCPServer(t *testing.T, refuse bool) *httptest.Server {
	return fakeMCPServerSession(t, refuse, true)
}

// fakeMCPServerSession with withSession=false mimics the service's stateless go-sdk handler: no
// Mcp-Session-Id is issued, and the client must send none.
func fakeMCPServerSession(t *testing.T, refuse, withSession bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svc-token" {
			w.WriteHeader(401)
			return
		}
		var msg struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&msg)
		switch msg.Method {
		case "initialize":
			if withSession {
				w.Header().Set("Mcp-Session-Id", "s1")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"superwitness","version":"0"}}}`, *msg.ID)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			if got := r.Header.Get("Mcp-Session-Id"); (withSession && got != "s1") || (!withSession && got != "") {
				w.WriteHeader(400)
				return
			}
			switch {
			case msg.Params.Name == "record_verdict" && refuse:
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"self_judgement"}],"isError":true}}`, *msg.ID)
			case msg.Params.Name == "record_verdict":
				if msg.Params.Arguments["idempotency_key"] != "k1" || msg.Params.Arguments["standard"] != "rubric:superwitness-canary@1" {
					t.Errorf("arguments = %v", msg.Params.Arguments)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"recorded\"}],\"structuredContent\":{\"id\":\"vrd_1\",\"judge\":\"prn_canary\",\"judge_kind\":\"grader\"}}}\n\n", *msg.ID)
			case msg.Params.Name == "get_run":
				doc, _ := json.Marshal(gradedDoc())
				text, _ := json.Marshal(string(doc))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":%s}]}}`, *msg.ID, text)
			}
		}
	}))
}

func TestMCPClientRecordsAndReadsBack(t *testing.T) {
	srv := fakeMCPServer(t, false)
	defer srv.Close()
	c := &MCPClient{URL: srv.URL, Tokens: staticToken("svc-token")}
	ctx := context.Background()
	v, err := c.RecordVerdict(ctx, VerdictInput{IdempotencyKey: "k1", SubjectKind: "run", SubjectRef: "superpipeline:brd_c/run_1",
		Standard: "rubric:superwitness-canary@1", Value: map[string]any{"score": 1.0}})
	if err != nil || v.ID != "vrd_1" || v.JudgeKind != "grader" {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	doc, err := c.GetRun(ctx, "brd_c", "run_1")
	if err != nil || len(doc.Verdicts) != 2 {
		t.Fatalf("doc=%+v err=%v", doc, err)
	}
}

func TestMCPClientSurfacesAToolRefusal(t *testing.T) {
	srv := fakeMCPServer(t, true)
	defer srv.Close()
	c := &MCPClient{URL: srv.URL, Tokens: staticToken("svc-token")}
	_, err := c.RecordVerdict(context.Background(), VerdictInput{IdempotencyKey: "k1"})
	if err == nil || !strings.Contains(err.Error(), "self_judgement") {
		t.Fatalf("err = %v", err)
	}
}

func TestMCPClientWorksWithoutASessionID(t *testing.T) {
	srv := fakeMCPServerSession(t, false, false)
	defer srv.Close()
	c := &MCPClient{URL: srv.URL, Tokens: staticToken("svc-token")}
	v, err := c.RecordVerdict(context.Background(), VerdictInput{IdempotencyKey: "k1", SubjectKind: "run",
		SubjectRef: "superpipeline:brd_c/run_1", Standard: "rubric:superwitness-canary@1", Value: map[string]any{"score": 1.0}})
	if err != nil || v.ID != "vrd_1" {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	if c.session != "" {
		t.Errorf("session = %q", c.session)
	}
}
