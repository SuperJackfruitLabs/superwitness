package join

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

var errUnresolved = errors.New("subject could not be resolved")

// Subjects answers "does this subject exist, and who executed it?" for the verdict rules.
type Subjects struct {
	Superpipeline source.Source
	AgentPod      source.Source
	Attempts      source.AttemptResolver
	Timeout       time.Duration
}

func (s *Subjects) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return DefaultTimeout
}

func (s *Subjects) ResolveRun(ctx context.Context, ref source.RunRef) (verdicts.Subject, error) {
	var sp, ap fetched
	var wg sync.WaitGroup
	wg.Go(func() { sp = fetchWithTimeout(ctx, s.Superpipeline, ref, s.timeout()) })
	wg.Go(func() { ap = fetchWithTimeout(ctx, s.AgentPod, ref, s.timeout()) })
	wg.Wait()

	if sp.status == source.StatusNotFound && ap.status == source.StatusNotFound {
		return verdicts.Subject{Found: false, Complete: true}, nil
	}
	if sp.status != source.StatusOK && ap.status != source.StatusOK {
		return verdicts.Subject{}, errUnresolved
	}
	subj := verdicts.Subject{Found: true, Complete: true}
	add := func(p string) {
		if p == "" {
			subj.Complete = false
			return
		}
		subj.Executors = append(subj.Executors, p)
	}
	switch {
	case sp.status == source.StatusOK && sp.frag.Run != nil:
		r := sp.frag.Run.Run
		add(r.AgentPrincipalID)
	case sp.status != source.StatusNotFound:
		subj.Complete = false
	}
	switch {
	case ap.status == source.StatusOK && ap.frag.Ledger != nil:
		for _, a := range ap.frag.Ledger.Attempts {
			add(a.AgentPrincipalID)
		}
	case ap.status != source.StatusNotFound:
		subj.Complete = false
	}
	if ap.frag.Execution != nil && ap.frag.ExecutionStatus == source.StatusOK && (sp.frag.Run == nil || ap.frag.Execution.MatchesRun(sp.frag.Run.Run)) {
		ids, complete := ap.frag.Execution.ContributorClosure()
		subj.Executors = append(subj.Executors, ids...)
		subj.Complete = subj.Complete && complete
		subj.Evidence = ap.frag.Execution
	} else {
		subj.Complete = false
	}
	if sp.frag.Work != nil && sp.frag.WorkStatus == source.StatusOK {
		reader := Joiner{AgentPod: s.AgentPod, Timeout: s.timeout()}
		for _, joined := range reader.readExports(ctx, sp.frag.Work) {
			if joined.Status == source.StatusOK && joined.Receipt != nil {
				subj.Exports = append(subj.Exports, *joined.Receipt)
			}
		}
	}
	return subj, nil
}

func (s *Subjects) ResolveAttempt(ctx context.Context, attemptID string) (verdicts.Subject, error) {
	cctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()
	if s.Attempts == nil {
		return verdicts.Subject{}, errUnresolved
	}
	link, st := s.Attempts.ResolveAttempt(cctx, attemptID)
	switch st {
	case source.StatusOK:
	case source.StatusNotFound:
		return verdicts.Subject{Found: false, Complete: true}, nil
	default:
		return verdicts.Subject{}, errUnresolved
	}
	ref, ok := link.Ref()
	if !ok {
		return verdicts.Subject{Found: true, Complete: false}, nil // not dispatched: no ledger to read
	}
	ap := fetchWithTimeout(ctx, s.AgentPod, ref, s.timeout())
	if ap.status != source.StatusOK || ap.frag.Ledger == nil {
		return verdicts.Subject{Found: true, Complete: false}, nil
	}
	for _, a := range ap.frag.Ledger.Attempts {
		if a.ID == attemptID {
			if a.AgentPrincipalID == "" {
				return verdicts.Subject{Found: true, Complete: false}, nil
			}
			subj := verdicts.Subject{Found: true, Complete: false, Executors: []string{a.AgentPrincipalID}}
			if ap.frag.Execution != nil && ap.frag.ExecutionStatus == source.StatusOK {
				scoped := *ap.frag.Execution
				scoped.Executions = nil
				for _, execution := range ap.frag.Execution.Executions {
					if execution.Binding.HubSessionID == a.SessionID {
						scoped.Executions = append(scoped.Executions, execution)
					}
				}
				if len(scoped.Executions) > 0 {
					ids, complete := scoped.ContributorClosure()
					subj.Executors = append(subj.Executors, ids...)
					subj.Complete = complete
					subj.Evidence = &scoped
				}
			}
			return subj, nil
		}
	}
	return verdicts.Subject{Found: true, Complete: false}, nil
}
