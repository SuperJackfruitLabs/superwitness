package join

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
)

func devSubjects(t *testing.T) (*Subjects, *fake.Dev) {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	return &Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d, Timeout: 200 * time.Millisecond}, d
}

func TestResolveRun(t *testing.T) {
	s, d := devSubjects(t)
	ref, _ := source.NewSuperpipelineRef(fake.DevBoard, fake.DevRun)
	subj, err := s.ResolveRun(context.Background(), ref)
	if err != nil || !subj.Found || subj.Complete || !slices.Contains(subj.Executors, fake.DevExecutor) {
		t.Errorf("subject = %+v, %v", subj, err)
	}

	d.SP.Fragment.Run.Run.AgentPrincipalID = "" // superpipeline sent agent_principal_id: null
	subj, _ = s.ResolveRun(context.Background(), ref)
	if subj.Complete {
		t.Error("a null agent_principal_id must leave the subject incomplete; agt_01 is never guessed")
	}

	d.SP.Status, d.AP.Status = source.StatusNotFound, source.StatusNotFound
	if subj, err := s.ResolveRun(context.Background(), ref); err != nil || subj.Found {
		t.Errorf("both not found: %+v %v", subj, err)
	}
	d.SP.Status, d.AP.Status = source.StatusTimeout, source.StatusNotFound
	if _, err := s.ResolveRun(context.Background(), ref); err == nil {
		t.Error("one timeout and one not_found cannot prove absence")
	}
}

func TestResolveAttempt(t *testing.T) {
	s, d := devSubjects(t)
	subj, err := s.ResolveAttempt(context.Background(), fake.DevAttempt)
	if err != nil || !subj.Found || subj.Complete || subj.Executors[0] != fake.DevExecutor {
		t.Errorf("subject = %+v, %v", subj, err)
	}
	if subj, err := s.ResolveAttempt(context.Background(), "attempt_nope"); err != nil || subj.Found {
		t.Errorf("unknown attempt: %+v %v", subj, err)
	}
	d.AP.Fragment.Ledger.Attempts[0].AgentPrincipalID = ""
	if subj, _ := s.ResolveAttempt(context.Background(), fake.DevAttempt); subj.Complete {
		t.Error("an attempt without an occupant principal must be incomplete")
	}
}

// Any executor without a principal id leaves the subject incomplete, from either source.
func TestResolveRunIncompleteWhenAnyExecutorUnmapped(t *testing.T) {
	s, d := devSubjects(t)
	ref, _ := source.NewSuperpipelineRef(fake.DevBoard, fake.DevRun)
	d.AP.Fragment.Ledger.Attempts[0].AgentPrincipalID = ""
	if subj, err := s.ResolveRun(context.Background(), ref); err != nil || !subj.Found || subj.Complete {
		t.Errorf("unmapped ledger occupant: %+v %v", subj, err)
	}
	d2, _ := fake.NewDev()
	s2 := &Subjects{Superpipeline: d2.SP, AgentPod: d2.AP, Attempts: d2, Timeout: 200 * time.Millisecond}
	d2.AP.Status = source.StatusUnavailable
	if subj, _ := s2.ResolveRun(context.Background(), ref); subj.Complete {
		t.Error("an unreadable ledger cannot prove the executor set is complete")
	}
}

func TestExecutionChildContributorClosure(t *testing.T) {
	s, d := devSubjects(t)
	parent := "prn_" + strings.Repeat("a", 20)
	child := "prn_" + strings.Repeat("b", 20)
	d.AP.Fragment.Execution = &source.ExecutionEnvelope{ExternalSource: "superpipeline", ExternalRunID: fake.DevRun, BoardID: d.AP.Fragment.Ledger.BoardID, CardID: d.AP.Fragment.Ledger.CardID, Executions: []source.ExecutionEvidence{{Binding: source.ExecutionBinding{PrincipalID: parent}, Contributors: source.ExecutionContributors{Closure: "complete"}, Children: []source.ExecutionChild{{ContributorPrincipalID: &child}}}}}
	d.AP.Fragment.ExecutionStatus = source.StatusOK
	ref, _ := source.NewSuperpipelineRef(fake.DevBoard, fake.DevRun)
	subj, err := s.ResolveRun(context.Background(), ref)
	if err != nil || !subj.Complete || !slices.Contains(subj.Executors, child) {
		t.Fatalf("%+v %v", subj, err)
	}
	d.AP.Fragment.Execution.Executions[0].Children[0].ContributorPrincipalID = nil
	subj, err = s.ResolveRun(context.Background(), ref)
	if err != nil || subj.Complete {
		t.Fatalf("unresolved child: %+v %v", subj, err)
	}
}

func TestWorkUsageDoesNotPromoteEstimateOrUnknownToReported(t *testing.T) {
	_, d := devSubjects(t)
	ref, _ := source.NewSuperpipelineRef(fake.DevBoard, fake.DevRun)
	cost := 1.25
	d.SP.Fragment.WorkStatus = source.StatusOK
	d.SP.Fragment.Work = &source.WorkEvidence{Usage: source.WorkUsage{Status: "estimated", EstimatedCostUSD: &cost}}
	j := Joiner{Superpipeline: d.SP, AgentPod: d.AP}
	doc, err := j.Build(context.Background(), ref)
	if err != nil || doc.Cost.Status != "estimated" || doc.Cost.USD == nil || doc.Cost.InputTokens != nil {
		t.Fatalf("%+v %v", doc.Cost, err)
	}
	d.SP.Fragment.Work.Usage.Status = "unknown"
	doc, err = j.Build(context.Background(), ref)
	if err != nil || doc.Cost.Status != "unknown" || doc.Cost.USD != nil {
		t.Fatalf("%+v %v", doc.Cost, err)
	}
}

func TestAttemptContributorClosureUsesHubSessionBinding(t *testing.T) {
	s, d := devSubjects(t)
	child := "prn_" + strings.Repeat("b", 20)
	execution := source.ExecutionEvidence{Binding: source.ExecutionBinding{PrincipalID: "prn_" + strings.Repeat("a", 20), HubSessionID: d.AP.Fragment.Ledger.Attempts[0].SessionID}, Contributors: source.ExecutionContributors{Closure: "complete"}, Children: []source.ExecutionChild{{ContributorPrincipalID: &child}}}
	d.AP.Fragment.Execution = &source.ExecutionEnvelope{Executions: []source.ExecutionEvidence{execution}}
	d.AP.Fragment.ExecutionStatus = source.StatusOK
	subj, err := s.ResolveAttempt(context.Background(), fake.DevAttempt)
	if err != nil || !subj.Complete || !slices.Contains(subj.Executors, child) {
		t.Fatalf("%+v %v", subj, err)
	}
	d.AP.Fragment.Execution.Executions[0].Binding.HubSessionID = "different_hub_session"
	subj, err = s.ResolveAttempt(context.Background(), fake.DevAttempt)
	if err != nil || subj.Complete || slices.Contains(subj.Executors, child) {
		t.Fatal("unjoined child may not establish attempt closure")
	}
}
