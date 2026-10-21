// Package agentpod reads the hub's evidence routes and, through
// GET /api/evidence/principals/:principalId, the kind on a principal's record.
package agentpod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

var (
	principalPattern = regexp.MustCompile(`^prn_[A-Za-z0-9_-]{1,64}$`)
	lookupKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`) // prn_… or a hub auth user id
)

type Adapter struct {
	BaseURL string
	Tokens  auth.TokenSource
	HC      *http.Client

	mu         sync.Mutex
	principals map[string]auth.PrincipalRecord
}

func New(baseURL string, tokens auth.TokenSource, hc *http.Client) *Adapter {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Adapter{BaseURL: strings.TrimRight(baseURL, "/"), Tokens: tokens, HC: hc, principals: map[string]auth.PrincipalRecord{}}
}

func (a *Adapter) Name() source.Name { return source.AgentPod }

func (a *Adapter) get(ctx context.Context, path string, out any) source.SourceStatus {
	tok, err := a.Tokens.Token(ctx)
	if err != nil {
		return source.TokenStatus(ctx, err)
	}
	_, st := source.GetJSON(ctx, a.HC, a.BaseURL+path, tok, out, isNotFound)
	if st == source.StatusUnauthorized {
		a.Tokens.Invalidate()
	}
	return st
}

func (a *Adapter) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	var body source.LedgerFragment
	st := a.get(ctx, "/api/evidence/runs/superpipeline/"+url.PathEscape(ref.RunID), &body)
	if st != source.StatusOK {
		return source.Fragment{}, st
	}
	if body.ExternalSource != string(source.Superpipeline) || body.ExternalRunID != ref.RunID {
		return source.Fragment{}, source.StatusUnavailable
	}
	if body.BoardID != nil && *body.BoardID != ref.BoardID {
		return source.Fragment{}, source.StatusNotFound // the run exists, but not on the board the caller named
	}
	f := source.Fragment{Source: source.AgentPod, FetchedAt: time.Now().UTC(), Version: body.AsOf, Ledger: &body, ExecutionStatus: source.StatusUnavailable}
	var execution source.ExecutionEnvelope
	est := a.get(ctx, "/api/evidence/runs/superpipeline/"+url.PathEscape(ref.RunID)+"/execution?schema_version="+url.QueryEscape(source.ExecutionSchemaV1), &execution)
	if est == source.StatusOK && execution.ExternalRunID == ref.RunID && execution.BoardID != nil && *execution.BoardID == ref.BoardID && len(execution.Executions) > 0 {
		valid := true
		for _, x := range execution.Executions {
			if x.Task.BoardID != ref.BoardID || execution.CardID == nil || x.Task.CardID != *execution.CardID || (body.CardID != nil && x.Task.CardID != *body.CardID) {
				valid = false
			}
		}
		if valid {
			f.Execution = &execution
			f.ExecutionStatus = source.StatusOK
		}
	}
	return f, source.StatusOK
}

func (a *Adapter) ResolveAttempt(ctx context.Context, attemptID string) (source.AttemptLink, source.SourceStatus) {
	if !source.ValidAttemptID(attemptID) {
		return source.AttemptLink{}, source.StatusNotFound
	}
	var link source.AttemptLink
	st := a.get(ctx, "/api/evidence/attempts/"+url.PathEscape(attemptID), &link)
	return link, st
}

func (a *Adapter) Lookup(ctx context.Context, key string) (auth.PrincipalRecord, error) {
	if !lookupKeyPattern.MatchString(key) {
		return auth.PrincipalRecord{}, auth.ErrPrincipalNotFound
	}
	a.mu.Lock()
	p, ok := a.principals[key]
	a.mu.Unlock()
	if ok {
		return p, nil
	}
	var body struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Suspended bool   `json:"suspended"`
	}
	switch st := a.get(ctx, "/api/evidence/principals/"+url.PathEscape(key), &body); st {
	case source.StatusOK:
	case source.StatusNotFound:
		return auth.PrincipalRecord{}, auth.ErrPrincipalNotFound
	default:
		return auth.PrincipalRecord{}, fmt.Errorf("%w: hub answered %s", auth.ErrLookupUnavailable, st)
	}
	k := auth.PrincipalKind(body.Kind)
	valid := principalPattern.MatchString(body.ID) && (k == auth.KindHuman || k == auth.KindAgent || k == auth.KindService)
	if !valid || (principalPattern.MatchString(key) && body.ID != key) {
		return auth.PrincipalRecord{}, fmt.Errorf("%w: malformed principal record", auth.ErrLookupUnavailable)
	}
	p = auth.PrincipalRecord{ID: body.ID, Kind: k, Suspended: body.Suspended}
	a.mu.Lock()
	a.principals[key] = p
	a.mu.Unlock()
	return p, nil
}

func (a *Adapter) Ping(ctx context.Context) source.SourceStatus {
	return source.PingURL(ctx, a.HC, a.BaseURL+"/health", "")
}

func isNotFound(b []byte) bool {
	var e struct {
		Error any `json:"error"`
	}
	return json.Unmarshal(b, &e) == nil && e.Error == "not_found"
}

// FetchExport reads the owner's persisted immutable export verification receipt.
// It never follows a Library URL supplied by a model or Pipeline reference.
func (a *Adapter) FetchExport(ctx context.Context, id string) (source.ExecutionExportReceipt, source.SourceStatus) {
	var receipt source.ExecutionExportReceipt
	if !exportIDPattern.MatchString(id) {
		return receipt, source.StatusUnavailable
	}
	st := a.get(ctx, "/api/evidence/exports/"+url.PathEscape(id)+"?schema_version="+url.QueryEscape(source.ExecutionExportSchemaV1), &receipt)
	if st == source.StatusOK && receipt.ExportID != id {
		return source.ExecutionExportReceipt{}, source.StatusUnavailable
	}
	return receipt, st
}

var exportIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
