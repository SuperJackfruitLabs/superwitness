package canary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TracesClient reads VictoriaTraces through its Jaeger-compatible query API.
type TracesClient struct {
	BaseURL     string
	TokenFile   string
	HTTP        *http.Client
	SearchLimit int
}

type jaegerResp struct {
	Data []jaegerTrace `json:"data"`
}

type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}

type jaegerSpan struct {
	SpanID        string     `json:"spanID"`
	OperationName string     `json:"operationName"`
	ProcessID     string     `json:"processID"`
	Tags          []jaegerKV `json:"tags"`
	Logs          []struct {
		Fields []jaegerKV `json:"fields"`
	} `json:"logs"`
}

type jaegerKV struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type jaegerProcess struct {
	ServiceName string     `json:"serviceName"`
	Tags        []jaegerKV `json:"tags"`
}

func (c TracesClient) limit() int {
	if c.SearchLimit > 0 {
		return c.SearchLimit
	}
	return 1000
}

func (c TracesClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := strings.TrimRight(c.BaseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if err := setBearer(req, c.TokenFile); err != nil {
		return nil, err
	}
	resp, err := orDefault(c.HTTP).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// tagString mirrors the service's unexported traces.tagString (not importable from here).
func tagString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

// decode keeps numbers as json.Number so large integer tags are not rendered as 1e+06.
func decode(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(out)
}

func toSpans(t jaegerTrace) []Span {
	out := make([]Span, 0, len(t.Spans))
	for _, sp := range t.Spans {
		p := t.Processes[sp.ProcessID]
		tags := map[string]string{}
		for _, kv := range sp.Tags {
			tags[kv.Key] = tagString(kv.Value)
		}
		for _, kv := range p.Tags {
			tags["process."+kv.Key] = tagString(kv.Value)
		}
		for i, l := range sp.Logs {
			for _, kv := range l.Fields {
				tags[fmt.Sprintf("event%d.%s", i, kv.Key)] = tagString(kv.Value)
			}
		}
		out = append(out, Span{TraceID: t.TraceID, Service: p.ServiceName, Operation: sp.OperationName, Tags: tags})
	}
	return out
}

// find returns which part of the span holds needle: "operation" or a tag key.
func (s Span) find(needle string) (string, bool) {
	if strings.Contains(s.Operation, needle) {
		return "operation", true
	}
	keys := make([]string, 0, len(s.Tags))
	for k := range s.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(s.Tags[k], needle) {
			return k, true
		}
	}
	return "", false
}

func (c TracesClient) Trace(ctx context.Context, traceID string) ([]Span, error) {
	raw, err := c.get(ctx, "/select/jaeger/api/traces/"+url.PathEscape(traceID), nil)
	if err != nil {
		return nil, err
	}
	var jr jaegerResp
	if err := decode(raw, &jr); err != nil {
		return nil, fmt.Errorf("trace %s: %w", traceID, err)
	}
	if len(jr.Data) == 0 {
		return nil, fmt.Errorf("trace %s not found", traceID)
	}
	var out []Span
	for _, t := range jr.Data {
		out = append(out, toSpans(t)...)
	}
	return out, nil
}

// FindByTag is flow 5's search: spans of one service carrying one tag value in the
// window, found even when Workers started a trace of their own. It returns the query it
// made, which becomes the proof's evidence.
func (c TracesClient) FindByTag(ctx context.Context, service, key, value string, start, end time.Time) ([]Span, string, error) {
	tags, err := json.Marshal(map[string]string{key: value})
	if err != nil {
		return nil, "", err
	}
	qs := url.Values{
		"service": {service},
		"tags":    {string(tags)},
		"start":   {strconv.FormatInt(start.UnixMicro(), 10)},
		"end":     {strconv.FormatInt(end.UnixMicro(), 10)},
		"limit":   {"20"},
	}
	query := "GET /select/jaeger/api/traces?" + qs.Encode()
	raw, err := c.get(ctx, "/select/jaeger/api/traces", qs)
	if err != nil {
		return nil, query, err
	}
	var jr jaegerResp
	if err := decode(raw, &jr); err != nil {
		return nil, query, fmt.Errorf("tag search: %w", err)
	}
	var out []Span
	for _, t := range jr.Data {
		out = append(out, toSpans(t)...)
	}
	return out, query, nil
}

// Scan searches every service's traces in the window. The Jaeger API requires a service,
// so the services list is the enumeration. That catches a harness's own spans too.
func (c TracesClient) Scan(ctx context.Context, start, end time.Time, needles map[string]string) (ScanResult, error) {
	res := newScanResult()
	raw, err := c.get(ctx, "/select/jaeger/api/services", nil)
	if err != nil {
		return res, err
	}
	var svc struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(raw, &svc); err != nil {
		return res, fmt.Errorf("services: %w", err)
	}
	if len(svc.Data) == 0 {
		return res, errors.New("VictoriaTraces reports no services in its index")
	}
	limit := c.limit()
	seen := map[string]bool{}
	for _, s := range svc.Data {
		q := url.Values{
			"service": {s},
			"start":   {strconv.FormatInt(start.UnixMicro(), 10)},
			"end":     {strconv.FormatInt(end.UnixMicro(), 10)},
			"limit":   {strconv.Itoa(limit)},
		}
		body, err := c.get(ctx, "/select/jaeger/api/traces", q)
		if err != nil {
			return res, err
		}
		var jr jaegerResp
		if err := decode(body, &jr); err != nil {
			return res, fmt.Errorf("traces for service %s: %w", s, err)
		}
		if len(jr.Data) >= limit {
			res.Truncated = true
		}
		for _, t := range jr.Data {
			if seen[t.TraceID] {
				continue
			}
			seen[t.TraceID] = true
			for _, sp := range toSpans(t) {
				res.Scanned++
				for label, n := range needles {
					if n == "" {
						continue
					}
					if key, ok := sp.find(n); ok {
						res.hit(label, redact(fmt.Sprintf("service=%s span=%s key=%s", sp.Service, sp.Operation, key), needles))
					}
				}
			}
		}
		// Backstop: the needle is in the response bytes but no span field held it.
		for label, n := range needles {
			if n != "" && res.Hits[label] == 0 && bytes.Contains(body, []byte(n)) {
				res.hit(label, redact("service="+s+" raw response, field not located", needles))
			}
		}
	}
	return res, nil
}
