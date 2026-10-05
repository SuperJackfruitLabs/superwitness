package verdicts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const (
	maxKeyBytes     = 200
	maxCommentRunes = 10000
)

var verdictID = regexp.MustCompile(`^vrd_[A-Za-z0-9]{1,64}$`)

var kinds = map[string]bool{"grader": true, "review": true, "eval": true, "calibration": true}

type Request struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Kind           string          `json:"kind,omitempty"`
	SubjectKind    SubjectKind     `json:"subject_kind"`
	SubjectRef     string          `json:"subject_ref"`
	Standard       string          `json:"standard"`
	Value          json.RawMessage `json:"value"`
	Comment        string          `json:"comment,omitempty"`
	EvidenceRefs   json.RawMessage `json:"evidence_refs,omitempty"`
	Supersedes     *string         `json:"supersedes,omitempty"`
	Judge          string          `json:"judge,omitempty"`
	JudgeKind      string          `json:"judge_kind,omitempty"`
}

// Error is a refusal with the HTTP status and stable code the API returns.
type Error struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func refuse(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

func retryable(code, msg string) *Error {
	return &Error{Status: 503, Code: code, Message: msg, Retryable: true}
}

// Subject says whether a verdict's subject exists and which principals executed it.
// Complete is false when any executor could not be mapped to a hub principal.
type Subject struct {
	Found     bool
	Complete  bool
	Executors []string
}

type SubjectResolver interface {
	ResolveRun(ctx context.Context, ref source.RunRef) (Subject, error)
	ResolveAttempt(ctx context.Context, attemptID string) (Subject, error)
}

type Service struct {
	Store    Store
	Subjects SubjectResolver
	Now      func() time.Time
	NewID    func() string
}

// JudgeKindOf maps the hub-signed principal kind to judge_kind.
func JudgeKindOf(k auth.PrincipalKind) (JudgeKind, bool) {
	switch k {
	case auth.KindHuman:
		return JudgeHuman, true
	case auth.KindAgent:
		return JudgeAgent, true
	case auth.KindService:
		return JudgeGrader, true
	}
	return "", false
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	var b [10]byte
	_, _ = rand.Read(b[:])
	return "vrd_" + hex.EncodeToString(b[:])
}

func storeDown() *Error {
	return retryable("store_unavailable", "the verdict store is unavailable; retry with the same idempotency_key")
}

func (s *Service) Record(ctx context.Context, caller auth.Principal, req Request) (Verdict, bool, error) {
	judgeKind, ok := JudgeKindOf(caller.Kind)
	if !ok {
		return Verdict{}, false, refuse(403, "judge_kind_unsupported", "principals of kind %q cannot record verdicts", caller.Kind)
	}
	if err := normalize(&req, caller, judgeKind); err != nil {
		return Verdict{}, false, err
	}

	existing, err := s.Store.GetByKey(ctx, req.IdempotencyKey)
	switch {
	case err == nil:
		return replay(existing, req, caller.ID)
	case !errors.Is(err, ErrNotFound):
		return Verdict{}, false, storeDown()
	}

	if err := s.checkSubject(ctx, req, caller.ID, judgeKind); err != nil {
		return Verdict{}, false, err
	}
	if err := s.checkStandard(ctx, req.Standard); err != nil {
		return Verdict{}, false, err
	}
	if err := s.checkSupersedes(ctx, req, caller.ID); errors.Is(err, ErrAlreadySuperseded) {
		return s.lostSupersedeRace(ctx, req, caller.ID)
	} else if err != nil {
		return Verdict{}, false, err
	}

	v := Verdict{
		ID: s.newID(), IdempotencyKey: req.IdempotencyKey, Kind: req.Kind,
		SubjectKind: req.SubjectKind, SubjectRef: req.SubjectRef,
		Judge: caller.ID, JudgeKind: judgeKind, Standard: req.Standard,
		Value: compact(req.Value), Comment: req.Comment, EvidenceRefs: compact(req.EvidenceRefs),
		Supersedes: req.Supersedes, CreatedAt: s.now().UTC(),
	}
	got, created, err := s.Store.Insert(ctx, v)
	switch {
	case errors.Is(err, ErrAlreadySuperseded):
		return s.lostSupersedeRace(ctx, req, caller.ID)
	case err != nil:
		return Verdict{}, false, storeDown()
	case !created: // lost a race on the same key
		return replay(got, req, caller.ID)
	}
	return got, true, nil
}

func normalize(req *Request, caller auth.Principal, judgeKind JudgeKind) error {
	if req.Judge != "" && req.Judge != caller.ID {
		return refuse(400, "judge_mismatch", "the judge is the authenticated caller; omit judge or set it to %s", caller.ID)
	}
	if req.JudgeKind != "" {
		return refuse(400, "judge_kind_not_accepted", "judge_kind is taken from the hub principal record, not from the request")
	}
	if n := len(req.IdempotencyKey); n == 0 || n > maxKeyBytes {
		return refuse(400, "invalid_idempotency_key", "idempotency_key is required, at most %d bytes", maxKeyBytes)
	}
	// Postgres text and jsonb reject NUL; refuse it here so bad input is a 400, not a retryable 503.
	if strings.ContainsRune(req.IdempotencyKey, 0) {
		return refuse(400, "invalid_idempotency_key", "idempotency_key must not contain NUL")
	}
	if req.Kind == "" {
		req.Kind = "grader"
		if judgeKind == JudgeHuman {
			req.Kind = "review"
		}
	}
	if !kinds[req.Kind] {
		return refuse(400, "invalid_kind", "kind must be one of grader, review, eval, calibration")
	}
	switch req.SubjectKind {
	case SubjectRun:
		ref, err := source.ParseRunRef(req.SubjectRef)
		if err != nil {
			return refuse(400, "invalid_subject", "%v", err)
		}
		req.SubjectRef = ref.String()
	case SubjectAttempt:
		if !source.ValidAttemptID(req.SubjectRef) {
			return refuse(400, "invalid_subject", "an attempt subject is attempt_<id>, got %q", req.SubjectRef)
		}
	case SubjectEvalCaseRun:
		if !caseRef.MatchString(req.SubjectRef) {
			return refuse(400, "invalid_subject", "an eval case run is case:<id>@sha256:<fingerprint>, got %q", req.SubjectRef)
		}
	default:
		return refuse(400, "invalid_subject_kind", "subject_kind must be run, attempt or eval_case_run")
	}
	if req.Standard == "" {
		return refuse(422, "missing_standard", "every verdict names a versioned standard: rubric:<id>@<version>, stage:<key> or case:<id>")
	}
	if !rubricStd.MatchString(req.Standard) && !stageStd.MatchString(req.Standard) && !caseStd.MatchString(req.Standard) {
		return refuse(422, "invalid_standard", "standard must be rubric:<id>@<version>, stage:<key> or case:<id>, got %q", req.Standard)
	}
	if err := ValidateValue(req.Value); err != nil {
		return refuse(400, "invalid_value", "%v", err)
	}
	if hasNUL(req.Value) {
		return refuse(400, "invalid_value", "value must not contain NUL (\\u0000)")
	}
	if hasLoneSurrogate(req.Value) {
		return refuse(400, "invalid_value", "value must not contain an unpaired UTF-16 surrogate escape")
	}
	if strings.ContainsRune(req.Comment, 0) {
		return refuse(400, "invalid_comment", "comment must not contain NUL")
	}
	if utf8.RuneCountInString(req.Comment) > maxCommentRunes {
		return refuse(400, "comment_too_long", "comment holds at most %d characters", maxCommentRunes)
	}
	if t := bytes.TrimSpace(req.EvidenceRefs); len(t) == 0 || string(t) == "null" {
		req.EvidenceRefs = json.RawMessage(`[]`)
	}
	if err := ValidateEvidenceRefs(req.EvidenceRefs); err != nil {
		return refuse(400, "invalid_evidence_refs", "%v", err)
	}
	if hasNUL(req.EvidenceRefs) {
		return refuse(400, "invalid_evidence_refs", "evidence_refs must not contain NUL (\\u0000)")
	}
	if hasLoneSurrogate(req.EvidenceRefs) {
		return refuse(400, "invalid_evidence_refs", "evidence_refs must not contain an unpaired UTF-16 surrogate escape")
	}
	if req.Supersedes != nil && *req.Supersedes == "" {
		req.Supersedes = nil
	}
	if req.Supersedes != nil && !verdictID.MatchString(*req.Supersedes) {
		return refuse(400, "invalid_supersedes", "supersedes must be a verdict id (vrd_...)")
	}
	return nil
}

func replay(existing Verdict, req Request, judge string) (Verdict, bool, error) {
	if sameRequest(existing, req, judge) {
		return existing, false, nil
	}
	return Verdict{}, false, refuse(409, "idempotency_conflict", "idempotency_key %q was already used for a different verdict", req.IdempotencyKey)
}

func sameRequest(v Verdict, req Request, judge string) bool {
	return v.Judge == judge && v.Kind == req.Kind && v.SubjectKind == req.SubjectKind && v.SubjectRef == req.SubjectRef &&
		v.Standard == req.Standard && v.Comment == req.Comment && equalPtr(v.Supersedes, req.Supersedes) &&
		jsonEqual(v.Value, req.Value) && jsonEqual(v.EvidenceRefs, req.EvidenceRefs)
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func compact(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

func (s *Service) checkSubject(ctx context.Context, req Request, judge string, kind JudgeKind) error {
	var subj Subject
	var err error
	switch req.SubjectKind {
	case SubjectEvalCaseRun:
		return nil
	case SubjectRun:
		ref, _ := source.ParseRunRef(req.SubjectRef)
		subj, err = s.Subjects.ResolveRun(ctx, ref)
	case SubjectAttempt:
		subj, err = s.Subjects.ResolveAttempt(ctx, req.SubjectRef)
	}
	if err != nil {
		return retryable("subject_unresolved", "could not confirm the subject and who executed it; retry")
	}
	if !subj.Found {
		return refuse(404, "subject_not_found", "%s %s was not found", req.SubjectKind, req.SubjectRef)
	}
	if kind == JudgeHuman {
		return nil
	}
	if slices.Contains(subj.Executors, judge) {
		return refuse(403, "self_judgement", "an agent may not judge a %s it executed", req.SubjectKind)
	}
	if !subj.Complete {
		return retryable("subject_unresolved", "the agents that executed this subject cannot all be mapped to hub principals yet, so a non-human verdict on it cannot be accepted")
	}
	return nil
}

func (s *Service) checkStandard(ctx context.Context, standard string) error {
	m := rubricStd.FindStringSubmatch(standard)
	if m == nil {
		return nil
	}
	version, _ := strconv.Atoi(m[2])
	ok, err := s.Store.RubricExists(ctx, m[1], version)
	if err != nil {
		return storeDown()
	}
	if !ok {
		return refuse(422, "unknown_rubric", "rubric %s version %d does not exist", m[1], version)
	}
	return nil
}

func (s *Service) checkSupersedes(ctx context.Context, req Request, judge string) error {
	if req.Supersedes == nil {
		return nil
	}
	prev, err := s.Store.Get(ctx, *req.Supersedes)
	switch {
	case errors.Is(err, ErrNotFound):
		return refuse(422, "supersedes_not_found", "verdict %s does not exist", *req.Supersedes)
	case err != nil:
		return storeDown()
	}
	if prev.SubjectKind != req.SubjectKind || prev.SubjectRef != req.SubjectRef || prev.Standard != req.Standard {
		return refuse(422, "supersedes_mismatch", "a correction must name the same subject and standard as the verdict it supersedes")
	}
	if prev.Judge != judge {
		return refuse(403, "not_original_judge", "only the judge of %s may supersede it", prev.ID)
	}
	has, err := s.Store.HasSuccessor(ctx, prev.ID)
	if err != nil {
		return storeDown()
	}
	if has {
		return ErrAlreadySuperseded
	}
	return nil
}

// lostSupersedeRace answers a correction whose target already has a successor. A concurrent
// retry of the same request may be that successor (it committed after this request's GetByKey
// missed), so re-read by idempotency key and replay; only a different request gets the 409.
func (s *Service) lostSupersedeRace(ctx context.Context, req Request, judge string) (Verdict, bool, error) {
	existing, err := s.Store.GetByKey(ctx, req.IdempotencyKey)
	switch {
	case err == nil:
		return replay(existing, req, judge)
	case !errors.Is(err, ErrNotFound):
		return Verdict{}, false, storeDown()
	}
	return Verdict{}, false, refuse(409, "already_superseded", "verdict %s was already superseded; supersede the latest verdict in its chain", *req.Supersedes)
}

// hasNUL reports whether any string or object key in the JSON text decodes to contain NUL.
// It scans tokens, not a decoded map, so a key shadowed by a later duplicate is still checked.
func hasNUL(raw json.RawMessage) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if str, ok := tok.(string); ok && strings.ContainsRune(str, 0) {
			return true
		}
	}
}

// hasLoneSurrogate reports whether any JSON string in raw holds a \uD800-\uDFFF escape that is
// not a high surrogate immediately followed by a low one. Postgres jsonb rejects those, while Go
// decodes them to U+FFFD, so the decoded-token scan in hasNUL cannot see them; this reads the raw text.
func hasLoneSurrogate(raw json.RawMessage) bool {
	inString, wantLow := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			inString = c == '"'
			continue
		}
		if c != '\\' || i+1 >= len(raw) || raw[i+1] != 'u' || i+5 >= len(raw) {
			if wantLow {
				return true // a high surrogate followed by anything but \u
			}
			switch c {
			case '"':
				inString = false
			case '\\':
				i++ // skip the escaped character, so \\ud800 is a backslash then text
			}
			continue
		}
		u, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return false // not valid JSON; the validators have already refused it
		}
		i += 5
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			if wantLow {
				return true
			}
			wantLow = true
		case u >= 0xDC00 && u <= 0xDFFF:
			if !wantLow {
				return true
			}
			wantLow = false
		default:
			if wantLow {
				return true
			}
		}
	}
	return wantLow
}
