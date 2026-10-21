// Package superpipeline reads GET /v1/boards/:boardId/runs/:runId/evidence.
package superpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type Adapter struct {
	BaseURL string
	Tokens  auth.TokenSource
	HC      *http.Client
}

func New(baseURL string, tokens auth.TokenSource, hc *http.Client) *Adapter {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Adapter{BaseURL: strings.TrimRight(baseURL, "/"), Tokens: tokens, HC: hc}
}

func (a *Adapter) Name() source.Name { return source.Superpipeline }

func (a *Adapter) Fetch(ctx context.Context, ref source.RunRef) (source.Fragment, source.SourceStatus) {
	tok, err := a.Tokens.Token(ctx)
	if err != nil {
		return source.Fragment{}, source.TokenStatus(ctx, err)
	}
	u := fmt.Sprintf("%s/v1/boards/%s/runs/%s/evidence", a.BaseURL, url.PathEscape(ref.BoardID), url.PathEscape(ref.RunID))
	var body source.RunFragment
	_, st := source.GetJSON(ctx, a.HC, u, tok, &body, isNotFound)
	if st == source.StatusUnauthorized {
		a.Tokens.Invalidate()
	}
	if st != source.StatusOK {
		return source.Fragment{}, st
	}
	if body.Run.ID != ref.RunID || body.Run.BoardID != ref.BoardID || body.Run.CardID != body.Card.ID {
		return source.Fragment{}, source.StatusUnavailable // an answer about another run is not an answer
	}
	f := source.Fragment{Source: source.Superpipeline, FetchedAt: time.Now().UTC(), Version: body.AsOf, Run: &body, WorkStatus: source.StatusUnavailable}
	var work source.WorkEvidence
	_, wst := source.GetJSON(ctx, a.HC, u+"-v1", tok, &work, isNotFound)
	if wst == source.StatusOK && work.Pipeline.Run.ID == ref.RunID && work.Pipeline.Run.BoardID == ref.BoardID && work.Pipeline.Run.CardID == body.Run.CardID {
		f.Work = &work
		f.WorkStatus = source.StatusOK
	}
	return f, source.StatusOK
}

func (a *Adapter) Ping(ctx context.Context) source.SourceStatus {
	return source.PingURL(ctx, a.HC, a.BaseURL+"/health", "")
}

func isNotFound(b []byte) bool {
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) != nil {
		return false
	}
	return e.Error.Code == "RUN_NOT_FOUND" || e.Error.Code == "BOARD_NOT_FOUND"
}

var _ source.Pinger = (*Adapter)(nil)
var _ source.Source = (*Adapter)(nil)
