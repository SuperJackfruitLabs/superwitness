package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// TranscriptsScope is the hub-signed scope a bearer token needs to read session content. A
// browser session needs none: only allowlisted people hold one.
const TranscriptsScope = "transcripts:read"

// Caller is who asks for content, and how they got in.
type Caller struct {
	Principal  auth.Principal
	ViaSession bool
}

type TranscriptRequest struct {
	Ref       source.RunRef
	AttemptID string // "" when the run has exactly one attempt
	SeqFrom   *int64 // nil: the attempt's first seq
	SeqTo     *int64 // nil: the attempt's last seq, or the session's live end
	Cursor    string // the hub's next_cursor
}

type TranscriptItemRequest struct {
	Ref       source.RunRef
	AttemptID string
	SeqFrom   int64 // the item's first seq
	Full      bool
	RangeFrom *int64 // the range of the page the item was shown in; nil with RangeTo: none
	RangeTo   *int64
}

// transcriptRead is what the audit line says about one read. It never holds content.
type transcriptRead struct {
	caller     Caller
	ref        source.RunRef
	attempt    string
	from, to   *int64
	item, full bool
	items      int
	redactions int
}

// transcriptMeta is all superwitness reads of the hub's body: the counts for the audit line.
type transcriptMeta struct {
	Items []json.RawMessage `json:"items"`
	Item  *struct {
		Redactions int `json:"redactions"`
	} `json:"item"`
	Redactions int `json:"redactions"`
}

// GetTranscript is one page of an attempt's transcript, as the hub answers it, plus attempt_id.
func (o *Ops) GetTranscript(ctx context.Context, c Caller, req TranscriptRequest) (json.RawMessage, error) {
	rd := transcriptRead{caller: c, ref: req.Ref, attempt: req.AttemptID}
	body, err := o.getTranscript(ctx, c, req, &rd)
	o.auditTranscript(rd, err)
	return body, err
}

// GetTranscriptItem is one item whole, by its first seq, plus attempt_id.
func (o *Ops) GetTranscriptItem(ctx context.Context, c Caller, req TranscriptItemRequest) (json.RawMessage, error) {
	rd := transcriptRead{caller: c, ref: req.Ref, attempt: req.AttemptID, item: true, full: req.Full}
	body, err := o.getTranscriptItem(ctx, c, req, &rd)
	o.auditTranscript(rd, err)
	return body, err
}

func (o *Ops) getTranscript(ctx context.Context, c Caller, req TranscriptRequest, rd *transcriptRead) (json.RawMessage, error) {
	if err := mayReadTranscripts(c); err != nil {
		return nil, err
	}
	a, err := o.transcriptAttempt(ctx, req.Ref, req.AttemptID)
	if err != nil {
		return nil, err
	}
	rd.attempt = a.ID
	from, to, err := attemptRange(a, req.SeqFrom, req.SeqTo)
	if err != nil {
		return nil, err
	}
	rd.from, rd.to = &from, to
	if o.Transcripts == nil {
		return nil, sourceUnavailable()
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()
	body, st := o.Transcripts.TranscriptPage(cctx, source.TranscriptQuery{SessionID: a.SessionID, SeqFrom: &from, SeqTo: to,
		Cursor: req.Cursor, OnBehalfOf: c.Principal.ID})
	if st != source.StatusOK {
		return nil, transcriptHubError(st)
	}
	out, m, err := withAttemptID(body, a.ID)
	if err != nil {
		return nil, err
	}
	rd.items, rd.redactions = len(m.Items), m.Redactions
	return out, nil
}

func (o *Ops) getTranscriptItem(ctx context.Context, c Caller, req TranscriptItemRequest, rd *transcriptRead) (json.RawMessage, error) {
	if err := mayReadTranscripts(c); err != nil {
		return nil, err
	}
	a, err := o.transcriptAttempt(ctx, req.Ref, req.AttemptID)
	if err != nil {
		return nil, err
	}
	rd.attempt = a.ID
	seq := req.SeqFrom
	rd.from = &seq
	if seq < a.StartSeq || (a.EndSeq != nil && seq > *a.EndSeq) {
		return nil, outsideAttempt(fmt.Sprintf("seq %d is not inside %s", seq, describeAttempt(a)))
	}
	var rangeFrom, rangeTo *int64
	if req.RangeFrom != nil || req.RangeTo != nil {
		f, t, err := attemptRange(a, req.RangeFrom, req.RangeTo)
		if err != nil {
			return nil, err
		}
		rangeFrom, rangeTo = &f, t
	}
	if o.Transcripts == nil {
		return nil, sourceUnavailable()
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()
	body, st := o.Transcripts.TranscriptItem(cctx, source.ItemQuery{SessionID: a.SessionID, SeqFrom: seq, Full: req.Full,
		RangeFrom: rangeFrom, RangeTo: rangeTo, OnBehalfOf: c.Principal.ID})
	if st != source.StatusOK {
		return nil, transcriptHubError(st)
	}
	out, m, err := withAttemptID(body, a.ID)
	if err != nil {
		return nil, err
	}
	if m.Item != nil {
		rd.items, rd.redactions = 1, m.Item.Redactions
	}
	return out, nil
}

// mayReadTranscripts admits a signed-in person, or a token granted transcripts:read.
func mayReadTranscripts(c Caller) error {
	if c.ViaSession || c.Principal.HasScope(TranscriptsScope) {
		return nil
	}
	return &APIError{Status: http.StatusForbidden, Code: "transcripts_forbidden",
		Message: "reading session content needs a signed-in session or a token granted " + TranscriptsScope}
}

// transcriptAttempt finds the attempt in the run's hub ledger: the named one, or the only one.
func (o *Ops) transcriptAttempt(ctx context.Context, ref source.RunRef, id string) (source.LedgerAttempt, error) {
	if id != "" && !source.ValidAttemptID(id) {
		return source.LedgerAttempt{}, badRequest("invalid_attempt_id", "an attempt id is attempt_<id>")
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()
	f, st := o.Join.AgentPod.Fetch(cctx, ref)
	switch {
	case st == source.StatusNotFound:
		return source.LedgerAttempt{}, attemptNotFound(ref, id)
	case st != source.StatusOK || f.Ledger == nil:
		return source.LedgerAttempt{}, sourceUnavailable()
	}
	atts := f.Ledger.Attempts
	var a source.LedgerAttempt
	switch {
	case id == "" && len(atts) == 1:
		a = atts[0]
	case id == "" && len(atts) == 0:
		return source.LedgerAttempt{}, attemptNotFound(ref, "")
	case id == "":
		return source.LedgerAttempt{}, &APIError{Status: http.StatusBadRequest, Code: "attempt_required",
			Message: fmt.Sprintf("%s has %d attempts; name one with attempt", ref, len(atts))}
	default:
		found := false
		for _, x := range atts {
			if x.ID == id {
				a, found = x, true
				break
			}
		}
		if !found {
			return source.LedgerAttempt{}, attemptNotFound(ref, id)
		}
	}
	if a.SessionID == "" {
		return source.LedgerAttempt{}, &APIError{Status: http.StatusNotFound, Code: "session_not_found",
			Message: "the hub records no session for " + a.ID}
	}
	return a, nil
}

// attemptRange fills an unset bound from the attempt and holds the range inside it. A live
// attempt (no end_seq yet) has no upper bound here: the hub answers a seq_to past the session's end.
func attemptRange(a source.LedgerAttempt, from, to *int64) (int64, *int64, error) {
	f := a.StartSeq
	if from != nil {
		f = *from
	}
	t := a.EndSeq
	if to != nil {
		v := *to
		t = &v
	}
	switch {
	case f < a.StartSeq:
		return 0, nil, outsideAttempt(fmt.Sprintf("seq_from %d is before %s", f, describeAttempt(a)))
	case a.EndSeq != nil && f > *a.EndSeq:
		return 0, nil, outsideAttempt(fmt.Sprintf("seq_from %d is after %s", f, describeAttempt(a)))
	case a.EndSeq != nil && t != nil && *t > *a.EndSeq:
		return 0, nil, outsideAttempt(fmt.Sprintf("seq_to %d is after %s", *t, describeAttempt(a)))
	case t != nil && f > *t:
		return 0, nil, outsideAttempt(fmt.Sprintf("seq_from %d is after seq_to %d", f, *t))
	}
	return f, t, nil
}

func describeAttempt(a source.LedgerAttempt) string {
	if a.EndSeq == nil {
		return fmt.Sprintf("%s (seq %d onwards, still running)", a.ID, a.StartSeq)
	}
	return fmt.Sprintf("%s (seq %d to %d)", a.ID, a.StartSeq, *a.EndSeq)
}

// withAttemptID adds "attempt_id" to the hub's JSON object as its first key and leaves every other
// byte as the hub sent it. It decodes only the counts the audit line needs.
func withAttemptID(body json.RawMessage, attemptID string) (json.RawMessage, transcriptMeta, error) {
	var m transcriptMeta
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || trimmed[0] != '{' || json.Unmarshal(trimmed, &m) != nil {
		return nil, transcriptMeta{}, sourceUnavailable()
	}
	id, _ := json.Marshal(attemptID)
	rest := bytes.TrimSpace(trimmed[1:])
	out := make([]byte, 0, len(trimmed)+len(id)+16)
	out = append(out, `{"attempt_id":`...)
	out = append(out, id...)
	if rest[0] != '}' {
		out = append(out, ',')
	}
	return append(out, rest...), m, nil
}

func attemptNotFound(ref source.RunRef, id string) *APIError {
	msg := ref.String() + " has no attempts the hub knows"
	if id != "" {
		msg = ref.String() + " has no attempt " + id
	}
	return &APIError{Status: http.StatusNotFound, Code: "attempt_not_found", Message: msg}
}

func outsideAttempt(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "range_outside_attempt", Message: msg}
}

func sourceUnavailable() *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "source_unavailable",
		Message: "the hub could not be read for this transcript; retry", Retryable: true}
}

// transcriptHubError maps the hub's answer to a transcript read.
func transcriptHubError(st source.SourceStatus) *APIError {
	switch st {
	case source.StatusNotFound:
		return &APIError{Status: http.StatusNotFound, Code: "session_not_found", Message: "the hub holds no such session or item"}
	case source.StatusTooLarge:
		return &APIError{Status: http.StatusRequestEntityTooLarge, Code: "item_too_large", Message: "the item is over the hub's 1 MiB"}
	case source.StatusUnauthorized:
		return &APIError{Status: http.StatusBadGateway, Code: "hub_refused",
			Message: "the hub refused superwitness's credential for transcripts; its grant needs " + TranscriptsScope}
	case source.StatusBadRange:
		return outsideAttempt("the range runs past the session's end")
	default:
		return sourceUnavailable()
	}
}

func callerOf(r *http.Request) Caller {
	p, _ := auth.PrincipalFrom(r.Context())
	return Caller{Principal: p, ViaSession: auth.ViaSession(r.Context())}
}

// seqParam reads an optional seq: absent is nil; anything but a whole number of 0 or more is 400.
func seqParam(v, name string) (*int64, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return nil, badRequest("invalid_seq", name+" must be a whole number of 0 or more")
	}
	return &n, nil
}

// writeRaw sends a JSON body as it is. Content is never cached anywhere: no-store, as writeJSON.
func writeRaw(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) getTranscript(w http.ResponseWriter, r *http.Request) {
	ref, ok := runRef(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, err := seqParam(q.Get("seq_from"), "seq_from")
	if err != nil {
		writeError(w, err)
		return
	}
	to, err := seqParam(q.Get("seq_to"), "seq_to")
	if err != nil {
		writeError(w, err)
		return
	}
	body, err := s.Ops.GetTranscript(r.Context(), callerOf(r), TranscriptRequest{Ref: ref, AttemptID: q.Get("attempt"),
		SeqFrom: from, SeqTo: to, Cursor: q.Get("cursor")})
	if err != nil {
		writeError(w, err)
		return
	}
	writeRaw(w, body)
}

func (s *Server) getTranscriptItem(w http.ResponseWriter, r *http.Request) {
	ref, ok := runRef(w, r)
	if !ok {
		return
	}
	seq, err := seqParam(chi.URLParam(r, "seqFrom"), "seq_from")
	if err == nil && seq == nil {
		err = badRequest("invalid_seq", "seq_from must be a whole number of 0 or more")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	q := r.URL.Query()
	rangeFrom, err := seqParam(q.Get("seq_from"), "seq_from")
	if err != nil {
		writeError(w, err)
		return
	}
	rangeTo, err := seqParam(q.Get("seq_to"), "seq_to")
	if err != nil {
		writeError(w, err)
		return
	}
	body, err := s.Ops.GetTranscriptItem(r.Context(), callerOf(r), TranscriptItemRequest{Ref: ref, AttemptID: q.Get("attempt"),
		SeqFrom: *seq, Full: q.Get("full") == "1", RangeFrom: rangeFrom, RangeTo: rangeTo})
	if err != nil {
		writeError(w, err)
		return
	}
	writeRaw(w, body)
}

// auditTranscript writes the read's transcript.read line. A no-op until the audit lands.
func (o *Ops) auditTranscript(rd transcriptRead, err error) {}
