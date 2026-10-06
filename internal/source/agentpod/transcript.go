package agentpod

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// What superwitness will read from a transcript route. A page is 200 items whose string fields
// the hub cuts at 16 KiB; an item is at most 1 MiB. Over the cap is unavailable, never a partial body.
const (
	maxTranscriptPage = 32 << 20
	maxTranscriptItem = 2 << 20
)

func (a *Adapter) TranscriptPage(ctx context.Context, q source.TranscriptQuery) (json.RawMessage, source.SourceStatus) {
	v := url.Values{}
	if q.SeqFrom != nil {
		v.Set("seq_from", strconv.FormatInt(*q.SeqFrom, 10))
	}
	if q.SeqTo != nil {
		v.Set("seq_to", strconv.FormatInt(*q.SeqTo, 10))
	}
	if q.Cursor != "" {
		v.Set("cursor", q.Cursor)
	}
	p := "/api/evidence/sessions/" + url.PathEscape(q.SessionID) + "/transcript"
	if len(v) > 0 {
		p += "?" + v.Encode()
	}
	return a.getContent(ctx, p, q.OnBehalfOf, maxTranscriptPage)
}

func (a *Adapter) TranscriptItem(ctx context.Context, q source.ItemQuery) (json.RawMessage, source.SourceStatus) {
	v := url.Values{}
	if q.Full {
		v.Set("full", "1")
	}
	if q.RangeFrom != nil {
		v.Set("seq_from", strconv.FormatInt(*q.RangeFrom, 10))
	}
	if q.RangeTo != nil {
		v.Set("seq_to", strconv.FormatInt(*q.RangeTo, 10))
	}
	p := "/api/evidence/sessions/" + url.PathEscape(q.SessionID) + "/transcript/items/" + strconv.FormatInt(q.SeqFrom, 10)
	if len(v) > 0 {
		p += "?" + v.Encode()
	}
	return a.getContent(ctx, p, q.OnBehalfOf, maxTranscriptItem)
}

// getContent GETs a transcript route and returns the body exactly as the hub sent it. The body
// is session content: nothing here logs it, or any error body, and nothing keeps it.
func (a *Adapter) getContent(ctx context.Context, path, onBehalfOf string, limit int64) (json.RawMessage, source.SourceStatus) {
	tok, err := a.Tokens.Token(ctx)
	if err != nil {
		return nil, source.TokenStatus(ctx, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.BaseURL+path, nil)
	if err != nil {
		return nil, source.StatusUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	if principalPattern.MatchString(onBehalfOf) {
		req.Header.Set("X-On-Behalf-Of", onBehalfOf)
	}
	resp, err := a.HC.Do(req)
	if err != nil {
		return nil, source.ClassifyErr(ctx, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, source.ClassifyErr(ctx, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if int64(len(body)) > limit || !json.Valid(body) {
			return nil, source.StatusUnavailable
		}
		return body, source.StatusOK
	case http.StatusUnauthorized, http.StatusForbidden:
		a.Tokens.Invalidate()
		return nil, source.StatusUnauthorized
	case http.StatusNotFound:
		if isNotFound(body) {
			return nil, source.StatusNotFound
		}
		return nil, source.StatusUnavailable // a hub without the route is not a missing session
	case http.StatusRequestEntityTooLarge:
		return nil, source.StatusTooLarge
	case http.StatusBadRequest:
		if hubErrorCode(body) == "bad_range" {
			return nil, source.StatusBadRange
		}
		return nil, source.StatusUnavailable
	default:
		return nil, source.StatusUnavailable
	}
}

// hubErrorCode is the hub's {"error": "<code>"}, or "".
func hubErrorCode(b []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(b, &e) != nil {
		return ""
	}
	return e.Error
}
