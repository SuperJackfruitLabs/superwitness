package canary

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const mcpProtocolVersion = "2025-06-18"

// VerdictInput is record_verdict's arguments: the POST /v1/verdicts body.
// No judge field: the server takes the judge from the token, never from the caller.
type VerdictInput struct {
	IdempotencyKey string         `json:"idempotency_key"`
	SubjectKind    string         `json:"subject_kind"`
	SubjectRef     string         `json:"subject_ref"`
	Standard       string         `json:"standard"`
	Value          map[string]any `json:"value"`
	Comment        string         `json:"comment"`
}

// MCPClient is the smallest MCP streamable-HTTP client the canary needs. It handles a
// JSON response and an event-stream response alike.
type MCPClient struct {
	URL    string
	Tokens TokenSource
	HTTP   *http.Client

	session string
	nextID  int
	ready   bool
}

func (c *MCPClient) ensure(ctx context.Context) error {
	if c.ready {
		return nil
	}
	if _, err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "superwitness-canary", "version": "1"},
	}, false); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	if _, err := c.rpc(ctx, "notifications/initialized", nil, true); err != nil {
		return fmt.Errorf("mcp initialized: %w", err)
	}
	c.ready = true
	return nil
}

func (c *MCPClient) rpc(ctx context.Context, method string, params any, notify bool) (json.RawMessage, error) {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	id := 0
	if !notify {
		c.nextID++
		id = c.nextID
		msg["id"] = id
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocolVersion)
	req.Header.Set("Authorization", "Bearer "+tok)
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	resp, err := orDefault(c.HTTP).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		c.session = s
	}
	if notify {
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d", method, resp.StatusCode)
		}
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", method, resp.StatusCode)
	}
	raw, err := readRPCPayload(resp, id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	if env.Error != nil {
		return nil, fmt.Errorf("%s: rpc error %d: %s", method, env.Error.Code, env.Error.Message)
	}
	return env.Result, nil
}

func readRPCPayload(resp *http.Response, id int) (json.RawMessage, error) {
	body := io.LimitReader(resp.Body, 32<<20)
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		b, err := io.ReadAll(body)
		return json.RawMessage(b), err
	}
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 32<<20)
	var data strings.Builder
	flush := func() (json.RawMessage, bool) {
		if data.Len() == 0 {
			return nil, false
		}
		raw := json.RawMessage(data.String())
		data.Reset()
		var probe struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(raw, &probe) == nil && string(probe.ID) == strconv.Itoa(id) {
			return raw, true
		}
		return nil, false
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if raw, ok := flush(); ok {
				return raw, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if raw, ok := flush(); ok {
		return raw, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("event stream ended without a response to request %d", id)
}

func (c *MCPClient) CallTool(ctx context.Context, name string, args, out any) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	res, err := c.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, false)
	if err != nil {
		return err
	}
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	text := ""
	for _, item := range r.Content {
		if item.Type == "text" {
			text = item.Text
			break
		}
	}
	if r.IsError {
		return fmt.Errorf("%s refused: %s", name, text)
	}
	if len(r.Structured) > 0 && string(r.Structured) != "null" {
		return json.Unmarshal(r.Structured, out)
	}
	if text == "" {
		return fmt.Errorf("%s returned no content", name)
	}
	return json.Unmarshal([]byte(text), out)
}

func (c *MCPClient) RecordVerdict(ctx context.Context, in VerdictInput) (Verdict, error) {
	var v Verdict
	if err := c.CallTool(ctx, "record_verdict", in, &v); err != nil {
		return Verdict{}, err
	}
	return v, nil
}

func (c *MCPClient) GetRun(ctx context.Context, boardID, runID string) (RunDoc, error) {
	var doc RunDoc
	err := c.CallTool(ctx, "get_run", map[string]string{"source": "superpipeline", "board_id": boardID, "run_id": runID}, &doc)
	return doc, err
}
