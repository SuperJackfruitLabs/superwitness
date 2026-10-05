package verdicts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func verdict(id, key string, sup *string, at time.Time) Verdict {
	return Verdict{ID: id, IdempotencyKey: key, Kind: "grader", SubjectKind: SubjectRun,
		SubjectRef: "superpipeline:brd_01/run_01", Judge: "prn_grader01", JudgeKind: JudgeAgent,
		Standard: "rubric:press@1", Value: json.RawMessage(`{"score":0.7}`), EvidenceRefs: json.RawMessage(`[]`),
		Supersedes: sup, CreatedAt: at}
}

func storeContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	v1, created, err := s.Insert(ctx, verdict("vrd_1", "k1", nil, t0))
	if err != nil || !created || v1.ID != "vrd_1" {
		t.Fatalf("insert: %+v %v %v", v1, created, err)
	}
	again, created, err := s.Insert(ctx, verdict("vrd_other", "k1", nil, t0))
	if err != nil || created || again.ID != "vrd_1" {
		t.Errorf("idempotent insert: %+v %v %v", again, created, err)
	}
	if _, err := s.GetByKey(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByKey missing: %v", err)
	}
	sup := "vrd_1"
	if _, _, err := s.Insert(ctx, verdict("vrd_2", "k2", &sup, t0.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Insert(ctx, verdict("vrd_3", "k3", &sup, t0.Add(2*time.Minute))); !errors.Is(err, ErrAlreadySuperseded) {
		t.Errorf("second supersede: %v", err)
	}
	if has, err := s.HasSuccessor(ctx, "vrd_1"); err != nil || !has {
		t.Errorf("HasSuccessor: %v %v", has, err)
	}
	attempt := verdict("vrd_4", "k4", nil, t0.Add(3*time.Minute))
	attempt.SubjectKind, attempt.SubjectRef = SubjectAttempt, "attempt_01"
	if _, _, err := s.Insert(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	cur, err := s.ListCurrent(ctx, []SubjectKey{{SubjectRun, "superpipeline:brd_01/run_01"}, {SubjectAttempt, "attempt_01"}})
	if err != nil || len(cur) != 2 || cur[0].ID != "vrd_2" || cur[1].ID != "vrd_4" {
		t.Errorf("ListCurrent = %+v %v", cur, err)
	}
	if ok, _ := s.RubricExists(ctx, "press", 1); ok {
		t.Error("rubric exists before insert")
	}
	r := Rubric{ID: "press", Version: 1, Name: "Press", Scale: json.RawMessage(`{"min":0,"max":1}`), Body: "…", CreatedBy: "prn_human01", CreatedAt: t0}
	if err := s.InsertRubric(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRubric(ctx, r); !errors.Is(err, ErrRubricExists) {
		t.Errorf("duplicate rubric: %v", err)
	}
	if ok, _ := s.RubricExists(ctx, "press", 1); !ok {
		t.Error("rubric missing after insert")
	}
	r2 := r
	r2.Version, r2.Scale = 2, json.RawMessage(`{"kind":"text"}`)
	if err := s.InsertRubric(ctx, r2); err != nil {
		t.Fatal(err)
	}
	rr := s.(RubricReader)
	all, err := rr.ListRubrics(ctx)
	if err != nil || len(all) != 2 || all[0].Version != 2 || all[1].Version != 1 || all[0].ID != "press" {
		t.Errorf("ListRubrics = %+v %v; want press@2 then press@1", all, err)
	}
	got, err := rr.GetRubric(ctx, "press", 1)
	if err != nil || got.Name != "Press" || !jsonEqual(got.Scale, json.RawMessage(`{"min":0,"max":1}`)) || got.Body != "…" {
		t.Errorf("GetRubric = %+v %v", got, err)
	}
	if _, err := rr.GetRubric(ctx, "press", 9); !errors.Is(err, ErrRubricNotFound) {
		t.Errorf("GetRubric missing: %v", err)
	}
	hist, err := s.(HistoryReader).ListSubject(ctx, SubjectKey{SubjectRun, "superpipeline:brd_01/run_01"})
	if err != nil || len(hist) != 2 || hist[0].ID != "vrd_1" || hist[1].ID != "vrd_2" {
		t.Errorf("ListSubject = %+v %v; want vrd_1 then vrd_2, the superseded one included", hist, err)
	}
}

func TestMemStoreContract(t *testing.T) { storeContract(t, NewMemStore()) }

func TestMemStoreFail(t *testing.T) {
	s := NewMemStore()
	s.Fail = ErrUnavailable
	if _, _, err := s.Insert(context.Background(), verdict("v", "k", nil, time.Now())); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}

// historyContract: past MaxHistory verdicts on one subject, ListSubject keeps the newest,
// oldest first, so the latest judgement is never the one dropped.
func historyContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	const ref = "superpipeline:brd_01/run_history"
	for i := 0; i <= MaxHistory; i++ { // MaxHistory+1 verdicts
		v := verdict(fmt.Sprintf("vrd_h%04d", i), fmt.Sprintf("kh%04d", i), nil, t0.Add(time.Duration(i)*time.Second))
		v.SubjectRef = ref
		if _, _, err := s.Insert(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := s.(HistoryReader).ListSubject(ctx, SubjectKey{SubjectRun, ref})
	if err != nil || len(hist) != MaxHistory {
		t.Fatalf("ListSubject: %d verdicts, %v; want %d", len(hist), err, MaxHistory)
	}
	if hist[0].ID != "vrd_h0001" || hist[MaxHistory-1].ID != fmt.Sprintf("vrd_h%04d", MaxHistory) {
		t.Errorf("ListSubject kept %s … %s; want vrd_h0001 … the newest, oldest first", hist[0].ID, hist[MaxHistory-1].ID)
	}
}

func TestMemStoreHistoryKeepsTheNewest(t *testing.T) { historyContract(t, NewMemStore()) }
