package canary

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// SPAttempt is one row of GET /v1/boards/:id/cards/:cardId/attempts (AttemptView).
type SPAttempt struct {
	RunID     string  `json:"runId"`
	StageKey  string  `json:"stageKey"`
	AgentID   string  `json:"agentId"`
	Status    string  `json:"status"`
	Outcome   *string `json:"outcome"`
	StartedAt string  `json:"startedAt"`
	EndedAt   *string `json:"endedAt"`
}

type SuperpipelineClient struct {
	BaseURL string
	Tokens  TokenSource
	HTTP    *http.Client
}

func (c SuperpipelineClient) CreateCard(ctx context.Context, boardID, title string, spec any) (string, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	}
	u := c.BaseURL + "/v1/boards/" + url.PathEscape(boardID) + "/cards"
	if err := doJSON(ctx, c.HTTP, http.MethodPost, u, tok, map[string]any{"title": title, "spec": spec}, http.StatusCreated, &out); err != nil {
		return "", fmt.Errorf("POST cards: %w", err)
	}
	if out.Card.ID == "" {
		return "", errors.New("POST cards: the response carried no card id")
	}
	return out.Card.ID, nil
}

func (c SuperpipelineClient) Attempts(ctx context.Context, boardID, cardID string) ([]SPAttempt, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Attempts []SPAttempt `json:"attempts"`
	}
	u := c.BaseURL + "/v1/boards/" + url.PathEscape(boardID) + "/cards/" + url.PathEscape(cardID) + "/attempts"
	if err := doJSON(ctx, c.HTTP, http.MethodGet, u, tok, nil, http.StatusOK, &out); err != nil {
		return nil, fmt.Errorf("GET attempts: %w", err)
	}
	return out.Attempts, nil
}
