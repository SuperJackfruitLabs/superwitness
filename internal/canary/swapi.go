package canary

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type SuperwitnessClient struct {
	BaseURL string
	Tokens  TokenSource
	HTTP    *http.Client
}

func (c SuperwitnessClient) GetRun(ctx context.Context, boardID, runID string) (RunDoc, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return RunDoc{}, err
	}
	var doc RunDoc
	u := c.BaseURL + "/v1/runs/superpipeline/" + url.PathEscape(boardID) + "/" + url.PathEscape(runID)
	if err := doJSON(ctx, c.HTTP, http.MethodGet, u, tok, nil, http.StatusOK, &doc); err != nil {
		return RunDoc{}, fmt.Errorf("GET run document: %w", err)
	}
	return doc, nil
}

// Logs reads the first page of GET …/logs. One page is enough: flow 8 needs
// one linked product line.
func (c SuperwitnessClient) Logs(ctx context.Context, boardID, runID string) ([]LogLine, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Logs []LogLine `json:"logs"`
	}
	u := c.BaseURL + "/v1/runs/superpipeline/" + url.PathEscape(boardID) + "/" + url.PathEscape(runID) + "/logs"
	if err := doJSON(ctx, c.HTTP, http.MethodGet, u, tok, nil, http.StatusOK, &out); err != nil {
		return nil, fmt.Errorf("GET run logs: %w", err)
	}
	return out.Logs, nil
}
