package verdicts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

var (
	human    = auth.Principal{ID: "prn_human01", Kind: auth.KindHuman}
	grader   = auth.Principal{ID: "prn_grader01", Kind: auth.KindAgent}
	executor = auth.Principal{ID: "prn_agent01", Kind: auth.KindAgent}
	canary   = auth.Principal{ID: "prn_canary", Kind: auth.KindService}
)

type fakeSubjects struct {
	run        Subject
	runErr     error
	attempt    Subject
	attemptErr error
	calls      int
}

func (f *fakeSubjects) ResolveRun(context.Context, source.RunRef) (Subject, error) {
	f.calls++
	return f.run, f.runErr
}

func (f *fakeSubjects) ResolveAttempt(context.Context, string) (Subject, error) {
	f.calls++
	return f.attempt, f.attemptErr
}

func newService(t *testing.T) (*Service, *MemStore, *fakeSubjects) {
	t.Helper()
	store := NewMemStore()
	if err := store.InsertRubric(context.Background(), Rubric{ID: "press", Version: 1, Name: "Press",
		Scale: json.RawMessage(`{}`), Body: "b", CreatedBy: "prn_human01"}); err != nil {
		t.Fatal(err)
	}
	subj := &fakeSubjects{
		run:     Subject{Found: true, Complete: true, Executors: []string{"prn_agent01"}},
		attempt: Subject{Found: true, Complete: true, Executors: []string{"prn_agent01"}},
	}
	n := 0
	svc := &Service{Store: store, Subjects: subj,
		Now:   func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
		NewID: func() string { n++; return "vrd_" + string(rune('a'+n-1)) }}
	return svc, store, subj
}

func req(key string) Request {
	return Request{IdempotencyKey: key, SubjectKind: SubjectRun, SubjectRef: "superpipeline:brd_01/run_01",
		Standard: "rubric:press@1", Value: json.RawMessage(`{"score":0.7}`)}
}

func wantCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != status || e.Code != code {
		t.Fatalf("err = %v, want %d %s", err, status, code)
	}
}

func TestRecordHumanReview(t *testing.T) {
	svc, _, _ := newService(t)
	v, created, err := svc.Record(context.Background(), human, req("k1"))
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if v.Judge != "prn_human01" || v.JudgeKind != JudgeHuman || v.Kind != "review" || string(v.EvidenceRefs) != "[]" ||
		v.ID != "vrd_a" || !v.CreatedAt.Equal(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("verdict = %+v", v)
	}
}

func TestServiceJudgeIsGrader(t *testing.T) {
	svc, _, _ := newService(t)
	v, _, err := svc.Record(context.Background(), canary, req("k1"))
	if err != nil || v.JudgeKind != JudgeGrader || v.Kind != "grader" {
		t.Fatalf("verdict = %+v, err = %v", v, err)
	}
}

func TestIdempotentReplay(t *testing.T) {
	svc, _, _ := newService(t)
	a, _, _ := svc.Record(context.Background(), grader, req("k1"))
	b, created, err := svc.Record(context.Background(), grader, req("k1"))
	if err != nil || created || b.ID != a.ID {
		t.Errorf("replay = %+v %v %v", b, created, err)
	}
}

// An idempotency key reused with a different value.
func TestIdempotencyKeyReusedWithDifferentValue(t *testing.T) {
	svc, _, _ := newService(t)
	_, _, _ = svc.Record(context.Background(), grader, req("k1"))
	r := req("k1")
	r.Value = json.RawMessage(`{"score":0.2}`)
	_, _, err := svc.Record(context.Background(), grader, r)
	wantCode(t, err, 409, "idempotency_conflict")
}

func TestIdempotencyKeyReusedByAnotherJudge(t *testing.T) {
	svc, _, _ := newService(t)
	_, _, _ = svc.Record(context.Background(), grader, req("k1"))
	_, _, err := svc.Record(context.Background(), human, req("k1"))
	wantCode(t, err, 409, "idempotency_conflict")
	if strings.Contains(err.Error(), "prn_grader01") {
		t.Error("the conflict revealed another principal's verdict")
	}
}

// An agent may not judge a run it executed.
func TestSelfJudgementRefused(t *testing.T) {
	svc, _, _ := newService(t)
	_, _, err := svc.Record(context.Background(), executor, req("k1"))
	wantCode(t, err, 403, "self_judgement")
	r := req("k2")
	r.SubjectKind, r.SubjectRef = SubjectAttempt, "attempt_01"
	_, _, err = svc.Record(context.Background(), executor, r)
	wantCode(t, err, 403, "self_judgement")
}

func TestAgentJudgeNeedsResolvedExecutors(t *testing.T) {
	svc, _, subj := newService(t)
	subj.run = Subject{Found: true, Complete: false}
	_, _, err := svc.Record(context.Background(), grader, req("k1"))
	wantCode(t, err, 503, "subject_unresolved")
	if _, created, err := svc.Record(context.Background(), human, req("k2")); err != nil || !created {
		t.Errorf("a human is not blocked by unresolved executors: %v", err)
	}
	_, _, err = svc.Record(context.Background(), canary, req("k3"))
	wantCode(t, err, 503, "subject_unresolved") // service callers fail closed too
}

func TestJudgeAndKindNotTakenFromBody(t *testing.T) {
	svc, _, _ := newService(t)
	r := req("k1")
	r.JudgeKind = "human"
	_, _, err := svc.Record(context.Background(), grader, r)
	wantCode(t, err, 400, "judge_kind_not_accepted")
	r = req("k2")
	r.Judge = "prn_human01"
	_, _, err = svc.Record(context.Background(), grader, r)
	wantCode(t, err, 400, "judge_mismatch")
	r = req("k3")
	r.Judge = "prn_grader01"
	if _, _, err := svc.Record(context.Background(), grader, r); err != nil {
		t.Errorf("judge equal to the caller is accepted: %v", err)
	}
}

func TestStandardRules(t *testing.T) {
	svc, _, _ := newService(t)
	cases := []struct {
		standard string
		status   int
		code     string
	}{
		{"", 422, "missing_standard"},
		{"rubric:press", 422, "invalid_standard"},
		{"press@1", 422, "invalid_standard"},
		{"rubric:press@2", 422, "unknown_rubric"},
	}
	for i, c := range cases {
		r := req("bad" + string(rune('a'+i)))
		r.Standard = c.standard
		_, _, err := svc.Record(context.Background(), human, r)
		wantCode(t, err, c.status, c.code)
	}
	for i, ok := range []string{"stage:review", "case:smoke"} {
		r := req("ok" + string(rune('a'+i)))
		r.Standard = ok
		if _, _, err := svc.Record(context.Background(), human, r); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

func TestValueRules(t *testing.T) {
	svc, _, _ := newService(t)
	for i, bad := range []string{`{}`, `{"score":1.5}`, `{"score":-0.1}`, `{"score":"high"}`, `{"score":0.5,"label":"x"}`,
		`{"decision":""}`, `{"mood":"ok"}`, `null`, `[]`, ``} {
		r := req("v" + string(rune('a'+i)))
		r.Value = json.RawMessage(bad)
		_, _, err := svc.Record(context.Background(), human, r)
		wantCode(t, err, 400, "invalid_value")
	}
	r := req("good")
	r.Value = json.RawMessage(`{"text":"clear and correct"}`)
	if _, _, err := svc.Record(context.Background(), human, r); err != nil {
		t.Error(err)
	}
}

func TestShapeRules(t *testing.T) {
	svc, _, _ := newService(t)
	r := req("")
	_, _, err := svc.Record(context.Background(), human, r)
	wantCode(t, err, 400, "invalid_idempotency_key")
	r = req("k1")
	r.Kind = "vibes"
	_, _, err = svc.Record(context.Background(), human, r)
	wantCode(t, err, 400, "invalid_kind")
	r = req("k2")
	r.SubjectRef = "superpipeline:brd 01/run_01"
	_, _, err = svc.Record(context.Background(), human, r)
	wantCode(t, err, 400, "invalid_subject")
	r = req("k3")
	r.SubjectKind = "card"
	_, _, err = svc.Record(context.Background(), human, r)
	wantCode(t, err, 400, "invalid_subject_kind")
	r = req("k4")
	r.Comment = strings.Repeat("x", 10001)
	_, _, err = svc.Record(context.Background(), human, r)
	wantCode(t, err, 400, "comment_too_long")
}

func TestSubjectResolution(t *testing.T) {
	svc, _, subj := newService(t)
	subj.run = Subject{Found: false, Complete: true}
	_, _, err := svc.Record(context.Background(), human, req("k1"))
	wantCode(t, err, 404, "subject_not_found")
	subj.runErr = errors.New("superpipeline timeout")
	_, _, err = svc.Record(context.Background(), human, req("k2"))
	wantCode(t, err, 503, "subject_unresolved")
}

func TestEvalCaseRunSkipsResolver(t *testing.T) {
	svc, _, subj := newService(t)
	r := req("k1")
	r.SubjectKind, r.SubjectRef = SubjectEvalCaseRun, "case:smoke@sha256:"+strings.Repeat("ab", 32)
	if _, _, err := svc.Record(context.Background(), grader, r); err != nil {
		t.Fatal(err)
	}
	if subj.calls != 0 {
		t.Errorf("resolver calls = %d", subj.calls)
	}
}

func TestSupersedeChain(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	v1, _, _ := svc.Record(ctx, grader, req("k1"))
	r := req("k2")
	r.Supersedes = &v1.ID
	v2, _, err := svc.Record(ctx, grader, r)
	if err != nil || *v2.Supersedes != v1.ID {
		t.Fatalf("supersede: %+v %v", v2, err)
	}
	r = req("k3")
	r.Supersedes = &v1.ID
	_, _, err = svc.Record(ctx, grader, r)
	wantCode(t, err, 409, "already_superseded")

	missing := "vrd_nope"
	r = req("k4")
	r.Supersedes = &missing
	_, _, err = svc.Record(ctx, grader, r)
	wantCode(t, err, 422, "supersedes_not_found")

	r = req("k5")
	r.Supersedes, r.Standard = &v2.ID, "stage:review"
	_, _, err = svc.Record(ctx, grader, r)
	wantCode(t, err, 422, "supersedes_mismatch")

	r = req("k6")
	r.Supersedes = &v2.ID
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 403, "not_original_judge")
}

// raceStore runs a hook once, just before the named store call, to let a concurrent request
// commit between this request's GetByKey miss and its supersede check or insert.
type raceStore struct {
	*MemStore
	beforeHasSuccessor func()
	beforeInsert       func() error // a non-nil error replaces the Insert result
}

func (r *raceStore) HasSuccessor(ctx context.Context, id string) (bool, error) {
	if f := r.beforeHasSuccessor; f != nil {
		r.beforeHasSuccessor = nil
		f()
	}
	return r.MemStore.HasSuccessor(ctx, id)
}

func (r *raceStore) Insert(ctx context.Context, v Verdict) (Verdict, bool, error) {
	if f := r.beforeInsert; f != nil {
		r.beforeInsert = nil
		if err := f(); err != nil {
			return Verdict{}, false, err
		}
	}
	return r.MemStore.Insert(ctx, v)
}

// Two identical supersede requests (same idempotency_key) race. The loser must get the
// winner's verdict back, never 409 already_superseded.
func TestConcurrentIdenticalSupersedeRetry(t *testing.T) {
	for _, at := range []string{"supersede check", "insert"} {
		t.Run(at, func(t *testing.T) {
			svc, mem, _ := newService(t)
			ctx := context.Background()
			v1, _, err := svc.Record(ctx, grader, req("k1"))
			if err != nil {
				t.Fatal(err)
			}
			r := req("k2")
			r.Supersedes = &v1.ID
			rs := &raceStore{MemStore: mem}
			svc.Store = rs
			var winner Verdict
			commitWinner := func() {
				var err error
				winner, _, err = (&Service{Store: mem, Subjects: svc.Subjects, Now: svc.Now, NewID: svc.NewID}).Record(ctx, grader, r)
				if err != nil {
					t.Fatalf("winner: %v", err)
				}
			}
			if at == "insert" {
				// Postgres reports the loser's INSERT as 23505 on verdicts_supersedes_key → ErrAlreadySuperseded.
				rs.beforeInsert = func() error { commitWinner(); return ErrAlreadySuperseded }
			} else {
				rs.beforeHasSuccessor = commitWinner
			}
			got, created, err := svc.Record(ctx, grader, r)
			if err != nil || created || got.ID != winner.ID || winner.ID == "" {
				t.Fatalf("loser = %+v created=%v err=%v; want the winner %s replayed", got, created, err, winner.ID)
			}
			// A different request superseding the same verdict still gets the 409.
			other := req("k3")
			other.Supersedes = &v1.ID
			_, _, err = svc.Record(ctx, grader, other)
			wantCode(t, err, 409, "already_superseded")
		})
	}
}

func TestStoreDownIsRetryable(t *testing.T) {
	svc, store, _ := newService(t)
	store.Fail = ErrUnavailable
	_, _, err := svc.Record(context.Background(), human, req("k1"))
	wantCode(t, err, 503, "store_unavailable")
	var e *Error
	if errors.As(err, &e); !e.Retryable {
		t.Error("store_unavailable must be retryable")
	}
}

func TestEvidenceRefs(t *testing.T) {
	svc, _, _ := newService(t)
	r := req("k1")
	r.EvidenceRefs = json.RawMessage(`["00f067aa0ba902b8",{"session_id":"acps_01","seq_from":1,"seq_to":9}]`)
	if _, _, err := svc.Record(context.Background(), human, r); err != nil {
		t.Fatal(err)
	}
	for i, bad := range []string{`{}`, `["xyz"]`, `[{"session_id":"acps_01","seq_from":9,"seq_to":1}]`,
		`[{"session_id":"acps_01","seq_from":1,"seq_to":2,"content":"leak"}]`} {
		r := req("e" + string(rune('a'+i)))
		r.EvidenceRefs = json.RawMessage(bad)
		_, _, err := svc.Record(context.Background(), human, r)
		wantCode(t, err, 400, "invalid_evidence_refs")
	}
}

// Postgres normalises jsonb, so a replay may differ from the stored bytes in spacing and key order.
func TestReplayComparesJSONSemantically(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	r := req("k1")
	r.EvidenceRefs = json.RawMessage(`[{"session_id":"acps_01","seq_from":1,"seq_to":9}]`)
	a, _, err := svc.Record(ctx, grader, r)
	if err != nil {
		t.Fatal(err)
	}
	r2 := req("k1")
	r2.Value = json.RawMessage(`{ "score": 0.70 }`)
	r2.EvidenceRefs = json.RawMessage(`[{"seq_to":9, "seq_from":1, "session_id":"acps_01"}]`)
	b, created, err := svc.Record(ctx, grader, r2)
	if err != nil || created || b.ID != a.ID {
		t.Errorf("reformatted replay = %+v %v %v", b, created, err)
	}
}

// Postgres rejects NUL in text and jsonb; that is bad input (400), not an outage (retryable 503).
func TestNULInputIsRefused(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	r := req("k\x001")
	_, _, err := svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_idempotency_key")
	r = req("k2")
	r.Comment = "a\x00b"
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_comment")
	r = req("k3")
	r.Value = json.RawMessage(`{"text":"a\u0000b"}`)
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_value")
	r = req("k4")
	r.Value = json.RawMessage(`{"text":"a\\u0000b"}`)
	if _, _, err = svc.Record(ctx, human, r); err != nil {
		t.Errorf("an escaped backslash followed by u0000 is not NUL: %v", err)
	}
}

// Postgres jsonb rejects an unpaired UTF-16 surrogate escape. Go decodes one to U+FFFD, so
// without a raw scan it would pass validation and fail at insert as a retryable 503 forever.
func TestLoneSurrogateIsRefused(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	for i, bad := range []string{`{"text":"\ud800"}`, `{"text":"\uDBFF"}`, `{"text":"\udc00"}`, `{"text":"a\ud800b"}`,
		`{"text":"\ud800A"}`, `{"text":"\ud800\\udc00"}`, `{"text":"\ude00\ud83d"}`, `{"text":"\ud800","text":"ok"}`} {
		r := req("v" + string(rune('a'+i)))
		r.Value = json.RawMessage(bad)
		_, _, err := svc.Record(ctx, human, r)
		wantCode(t, err, 400, "invalid_value")
	}
	r := req("e1")
	r.EvidenceRefs = json.RawMessage(`[{"session_id":"acps_\udfff","seq_from":1,"seq_to":2}]`)
	_, _, err := svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_evidence_refs")

	for i, ok := range []string{`{"text":"😀"}`, `{"text":"😀 ok"}`, `{"text":"\\ud800"}`, `{"text":"é\"\\"}`} {
		r := req("ok" + string(rune('a'+i)))
		r.Value = json.RawMessage(ok)
		if _, _, err := svc.Record(ctx, human, r); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

func TestNULBypassesAndSupersedesFormat(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	r := req("s1")
	bad := "vrd_a\x00"
	r.Supersedes = &bad
	_, _, err := svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_supersedes")
	bad = "not an id"
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_supersedes")

	r = req("d1") // a duplicate key shadows the NUL in a decoded map but not in the stored text
	r.Value = json.RawMessage(`{"text":"a\u0000","text":"b"}`)
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_value")

	r = req("e1")
	r.EvidenceRefs = json.RawMessage(`[{"session_id":"a\u0000","seq_from":1,"seq_to":2}]`)
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_evidence_refs")
	r.EvidenceRefs = json.RawMessage(`[{"session_id":"a\u0000","session_id":"b","seq_from":1,"seq_to":2}]`)
	_, _, err = svc.Record(ctx, human, r)
	wantCode(t, err, 400, "invalid_evidence_refs")
}
