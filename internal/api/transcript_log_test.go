package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/mcp"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/agentpod"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
)

// contentKeys are the item fields that hold session content.
var contentKeys = map[string]bool{"text": true, "input": true, "output": true, "title": true, "options": true, "message": true, "head": true}

// contentNeedles is every string in the development transcript's content fields, cut to its first
// 24 characters, both raw and as a JSON log line would escape it.
func contentNeedles(t *testing.T, d *fake.Dev) []string {
	t.Helper()
	var items []any
	for _, seq := range []int64{1, 2, 3, 5, 7, 9} {
		b, st := d.TranscriptItem(context.Background(), source.ItemQuery{SessionID: fake.DevSession, SeqFrom: seq, Full: true})
		if st != source.StatusOK {
			t.Fatalf("dev item %d: %s", seq, st)
		}
		var v struct {
			Item map[string]any `json:"item"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		items = append(items, v.Item)
	}
	var out []string
	var walk func(v any, content bool)
	walk = func(v any, content bool) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				walk(c, content || contentKeys[k])
			}
		case []any:
			for _, c := range x {
				walk(c, content)
			}
		case string:
			if content && len(x) >= 8 {
				n := x[:min(24, len(x))]
				esc, _ := json.Marshal(n)
				out = append(out, n, string(esc[1:len(esc)-1]))
			}
		}
	}
	for _, it := range items {
		walk(it, false)
	}
	if len(out) < 20 {
		t.Fatalf("only %d needles: the walk is broken", len(out))
	}
	return append(out, "[redacted:")
}

type bearerRT struct {
	token string
	base  http.RoundTripper
}

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// TestTranscriptContentNeverLogged captures every log line written while transcripts are read
// (superwitness's logger, slog's default and the log package) through the real hub client, over
// HTTP and MCP, on success and on failure, and fails if any piece of content appears.
func TestTranscriptContentNeverLogged(t *testing.T) {
	var buf syncBuffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prevDefault, prevOut, prevFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(logger)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	var garble atomic.Bool
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/api/evidence/sessions/" + fake.DevSession + "/transcript"
		switch {
		case garble.Load():
			// A hub error whose body echoes the prompt.
			page, _ := d.TranscriptPage(r.Context(), source.TranscriptQuery{SessionID: fake.DevSession})
			w.WriteHeader(500)
			fmt.Fprintf(w, `{"error":"internal","detail":%q}`, page)
		case r.URL.Path == prefix:
			page, _ := d.TranscriptPage(r.Context(), source.TranscriptQuery{SessionID: fake.DevSession})
			_, _ = w.Write(page)
		case strings.HasPrefix(r.URL.Path, prefix+"/items/"):
			seq, _ := strconv.ParseInt(path.Base(r.URL.Path), 10, 64)
			item, st := d.TranscriptItem(r.Context(), source.ItemQuery{SessionID: fake.DevSession, SeqFrom: seq, Full: true})
			if st != source.StatusOK {
				w.WriteHeader(404)
				fmt.Fprint(w, `{"error":"not_found"}`)
				return
			}
			_, _ = w.Write(item)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	}))
	defer hub.Close()

	a := newAppServerWith(t, nil, func(s *api.Server) {
		s.Logger = logger
		s.Ops.Logger = logger
		s.Ops.Transcripts = agentpod.New(hub.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
		s.MCP = mcp.NewHandler(s.Ops, "test")
	})

	for _, c := range []struct{ path, bearer, cookie string }{
		{txPath, "", "cookie-human01"},
		{txPath + "?seq_from=3&seq_to=4", "dev:prn_grader01:agent:transcripts:read", ""},
		{txPath + "/items/3?full=1", "", "cookie-human01"},
		{txPath + "/items/7", "dev:prn_grader01:agent:transcripts:read", ""},
		{txPath + "/items/4", "", "cookie-human01"},          // the hub's 404
		{txPath, "dev:prn_grader01:agent:evidence:read", ""}, // refused
	} {
		if resp, b := a.call(t, "GET", c.path, c.bearer, c.cookie, "", ""); resp.StatusCode >= 500 {
			t.Fatalf("%s: %d %s", c.path, resp.StatusCode, b)
		}
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: a.srv.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerRT{"dev:prn_grader01:agent:transcripts:read", http.DefaultTransport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "get_transcript",
		Arguments: map[string]any{"board_id": "brd_01", "run_id": "run_01"}})
	if err != nil || res.IsError {
		t.Fatalf("get_transcript: %v %+v", err, res)
	}
	cs.Close()

	garble.Store(true)
	if resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", ""); resp.StatusCode != 503 {
		t.Fatalf("a hub error echoing content: %d %s", resp.StatusCode, b)
	}

	logs := buf.String()
	if n := strings.Count(logs, `"msg":"transcript.read"`); n != 8 {
		t.Fatalf("%d transcript.read lines, want 8 (six HTTP reads, one MCP read, one hub error): the capture is broken\n%s", n, logs)
	}
	for _, needle := range contentNeedles(t, d) {
		if strings.Contains(logs, needle) {
			t.Errorf("log output holds transcript content %q", needle)
		}
	}
}
