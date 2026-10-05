// Package traces reads spans for a run from VictoriaTraces' Jaeger-compatible query API.
package traces

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// DefaultServices are the service.name values that emit spans about a run.
var DefaultServices = []string{"agentpod-hub", "agentpod-node-agent", "superpipeline-api"}

type Adapter struct {
	BaseURL  string
	Bearer   string
	HC       *http.Client
	Services []string
	Lookback time.Duration
	Now      func() time.Time
}

func New(baseURL, bearer string, hc *http.Client) *Adapter {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Adapter{BaseURL: strings.TrimRight(baseURL, "/"), Bearer: bearer, HC: hc,
		Services: DefaultServices, Lookback: 7 * 24 * time.Hour, Now: time.Now}
}

func (a *Adapter) Name() source.Name { return source.Traces }

func (a *Adapter) SearchURL(service, runID string) string {
	now := a.Now().UTC()
	tags, _ := json.Marshal(map[string]string{"run.id": runID})
	q := url.Values{}
	q.Set("service", service)
	q.Set("tags", string(tags))
	q.Set("start", strconv.FormatInt(now.Add(-a.Lookback).UnixMicro(), 10))
	q.Set("end", strconv.FormatInt(now.Add(time.Hour).UnixMicro(), 10))
	q.Set("limit", "100")
	return a.BaseURL + "/select/jaeger/api/traces?" + q.Encode()
}

type jaegerResponse struct {
	Data []jaegerTrace `json:"data"`
}

type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}

type jaegerSpan struct {
	TraceID       string      `json:"traceID"`
	SpanID        string      `json:"spanID"`
	OperationName string      `json:"operationName"`
	References    []jaegerRef `json:"references"`
	StartTime     int64       `json:"startTime"`
	Duration      int64       `json:"duration"`
	Tags          []jaegerTag `json:"tags"`
	ProcessID     string      `json:"processID"`
}

type jaegerRef struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type jaegerTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type jaegerProcess struct {
	ServiceName string `json:"serviceName"`
}

func (a *Adapter) ListSpans(ctx context.Context, ref source.RunRef) ([]source.Span, source.SourceStatus) {
	type result struct {
		traces []jaegerTrace
		st     source.SourceStatus
	}
	results := make([]result, len(a.Services))
	var wg sync.WaitGroup
	for i, svc := range a.Services {
		wg.Go(func() {
			var body jaegerResponse
			_, st := source.GetJSON(ctx, a.HC, a.SearchURL(svc, ref.RunID), a.Bearer, &body,
				func([]byte) bool { return true }) // a 404 from a never-seen service means no traces
			if st == source.StatusNotFound {
				st = source.StatusOK
			}
			results[i] = result{traces: body.Data, st: st}
		})
	}
	wg.Wait()

	worst := source.StatusOK
	for _, r := range results {
		worst = worse(worst, r.st)
	}
	if worst != source.StatusOK {
		return nil, worst
	}

	seen := map[string]bool{}
	spans := []source.Span{}
	for _, r := range results {
		for _, tr := range r.traces {
			for _, s := range tr.Spans {
				sp := convert(s, tr.Processes)
				key := sp.TraceID + "/" + sp.SpanID
				if seen[key] {
					continue
				}
				seen[key] = true
				spans = append(spans, sp)
			}
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if !spans[i].Start.Equal(spans[j].Start) {
			return spans[i].Start.Before(spans[j].Start)
		}
		if spans[i].TraceID != spans[j].TraceID {
			return spans[i].TraceID < spans[j].TraceID
		}
		return spans[i].SpanID < spans[j].SpanID
	})
	return spans, source.StatusOK
}

func (a *Adapter) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	spans, st := a.ListSpans(ctx, ref)
	if st != source.StatusOK {
		return source.Fragment{}, st
	}
	return source.Fragment{Source: source.Traces, FetchedAt: time.Now().UTC(),
		Traces: &source.TracesFragment{TraceIDs: source.TraceIDsOf(spans), Spans: spans}}, source.StatusOK
}

func (a *Adapter) Ping(ctx context.Context) source.SourceStatus {
	return source.PingURL(ctx, a.HC, a.BaseURL+"/health", a.Bearer)
}

func convert(s jaegerSpan, procs map[string]jaegerProcess) source.Span {
	traceID := padHex(s.TraceID, 32)
	attrs := make(map[string]string, len(s.Tags))
	for _, t := range s.Tags {
		attrs[t.Key] = tagString(t.Value)
	}
	parent := ""
	for _, r := range s.References {
		if r.RefType == "CHILD_OF" && padHex(r.TraceID, 32) == traceID {
			parent = padHex(r.SpanID, 16)
			break
		}
	}
	return source.Span{
		TraceID: traceID, SpanID: padHex(s.SpanID, 16), ParentSpanID: parent,
		Name: s.OperationName, Service: procs[s.ProcessID].ServiceName,
		Start: time.UnixMicro(s.StartTime).UTC(), DurationMS: float64(s.Duration) / 1000, Attributes: attrs,
	}
}

// padHex restores leading zeros that Jaeger's JSON model may strip, so ids match OTLP's.
func padHex(id string, n int) string {
	id = strings.ToLower(id)
	if len(id) < n {
		return strings.Repeat("0", n-len(id)) + id
	}
	return id
}

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

func worse(a, b source.SourceStatus) source.SourceStatus {
	rank := map[source.SourceStatus]int{source.StatusOK: 0, source.StatusNotFound: 1, source.StatusUnavailable: 2,
		source.StatusTimeout: 3, source.StatusUnauthorized: 4}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
