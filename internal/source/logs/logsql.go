// Package logs reads a run's log lines from VictoriaLogs with LogsQL.
package logs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const Lookback = "7d"

var ErrInvalidLevel = errors.New("level must be one of debug, info, warn, error")

var levelRanges = map[string][2]int{"debug": {5, 8}, "info": {9, 12}, "warn": {13, 16}, "error": {17, 24}}

type Client struct {
	BaseURL string
	Bearer  string
	HC      *http.Client
}

func NewClient(baseURL, bearer string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Bearer: bearer, HC: hc}
}

// Query runs one LogsQL query and returns each JSON line as field → string.
func (c *Client) Query(ctx context.Context, query string) ([]map[string]string, source.SourceStatus) {
	form := url.Values{"query": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/select/logsql/query", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, source.StatusUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Bearer)
	}
	resp, err := c.HC.Do(req)
	if err != nil {
		return nil, source.ClassifyErr(ctx, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, source.StatusUnauthorized
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, source.StatusUnavailable
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 32<<20))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	rows := []map[string]string{}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			return nil, source.StatusUnavailable
		}
		row := make(map[string]string, len(raw))
		for k, v := range raw {
			row[k] = fmt.Sprint(v)
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, source.ClassifyErr(ctx, err)
	}
	return rows, source.StatusOK
}

func (c *Client) Ping(ctx context.Context) source.SourceStatus {
	return source.PingURL(ctx, c.HC, c.BaseURL+"/health", c.Bearer)
}

// Quote produces a LogsQL double-quoted string (Go escaping, which LogsQL accepts).
func Quote(s string) string { return strconv.Quote(s) }

// RunFilter matches lines that carry the run's id or belong to one of its traces.
func RunFilter(ref source.RunRef) string {
	parts := []string{`"run.id":=` + Quote(ref.RunID)}
	var ids []string
	for _, id := range ref.TraceIDs {
		if source.ValidTraceID(id) {
			ids = append(ids, Quote(id))
		}
	}
	if len(ids) > 0 {
		parts = append(parts, "trace_id:in("+strings.Join(ids, ",")+")")
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func LevelFilter(level string) (string, error) {
	r, ok := levelRanges[level]
	if !ok {
		return "", ErrInvalidLevel
	}
	return fmt.Sprintf("(severity_number:range[%d, %d] or severity_text:i(%s))", r[0], r[1], Quote(level)), nil
}

func BuildListQuery(ref source.RunRef, q source.LogQuery) (string, error) {
	query := "_time:" + Lookback + " " + RunFilter(ref)
	if q.Level != "" {
		lf, err := LevelFilter(q.Level)
		if err != nil {
			return "", err
		}
		query += " " + lf
	}
	return fmt.Sprintf("%s | sort by (_time) | offset %d | limit %d", query, q.Offset, q.Limit), nil
}

func RowTime(row map[string]string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, row["_time"])
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func RowLine(row map[string]string) source.LogLine {
	svc := row["service.name"]
	if svc == "" {
		svc = "unknown"
	}
	return source.LogLine{At: RowTime(row), Service: svc, Level: levelOf(row), Message: row["_msg"],
		TraceID: row["trace_id"], SpanID: row["span_id"], RunID: row["run.id"]}
}

func levelOf(row map[string]string) string {
	switch t := strings.ToLower(row["severity_text"]); {
	case strings.HasPrefix(t, "err"), t == "fatal", t == "critical":
		return "error"
	case strings.HasPrefix(t, "warn"):
		return "warn"
	case t == "info", t == "debug":
		return t
	}
	if n, err := strconv.Atoi(row["severity_number"]); err == nil {
		for _, l := range []string{"debug", "info", "warn", "error"} {
			if r := levelRanges[l]; n >= r[0] && n <= r[1] {
				return l
			}
		}
	}
	return "unknown"
}
