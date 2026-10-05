package join

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const DefaultTimeout = 2 * time.Second

var ErrRunNotFound = errors.New("run not found")

type VerdictReader interface {
	ListCurrent(ctx context.Context, subjects []verdicts.SubjectKey) ([]verdicts.Verdict, error)
}

type Joiner struct {
	Superpipeline source.Source
	AgentPod      source.Source
	Traces        source.Source
	Logs          source.Source
	Errors        source.Source
	Verdicts      VerdictReader
	Principals    auth.PrincipalLookup // hub principals route: gate deciders by prn_ or hub sub; nil shows "unknown"
	Timeout       time.Duration
	Observe       func(source.Name, source.SourceStatus) // own telemetry; may be nil
}

type fetched struct {
	frag   source.Fragment
	status source.SourceStatus
}

func (j *Joiner) timeout() time.Duration {
	if j.Timeout > 0 {
		return j.Timeout
	}
	return DefaultTimeout
}

func fetchWithTimeout(ctx context.Context, s source.Source, ref source.RunRef, d time.Duration) fetched {
	if s == nil {
		return fetched{status: source.StatusUnavailable}
	}
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	ch := make(chan fetched, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fetched{status: source.StatusUnavailable}
			}
		}()
		f, st := s.Fetch(cctx, ref)
		ch <- fetched{frag: f, status: st}
	}()
	select {
	case r := <-ch:
		switch r.status {
		case source.StatusOK, source.StatusNotFound, source.StatusUnauthorized, source.StatusTimeout, source.StatusUnavailable:
		default:
			r.status = source.StatusUnavailable
		}
		if r.status != source.StatusOK && errors.Is(cctx.Err(), context.DeadlineExceeded) {
			r.status = source.StatusTimeout
		}
		return r
	case <-cctx.Done():
		return fetched{status: source.StatusTimeout}
	}
}

func (j *Joiner) fetch(ctx context.Context, name source.Name, s source.Source, ref source.RunRef) fetched {
	r := fetchWithTimeout(ctx, s, ref, j.timeout())
	if j.Observe != nil {
		j.Observe(name, r.status)
	}
	return r
}

func (j *Joiner) Build(ctx context.Context, ref source.RunRef) (RunDocument, error) {
	var sp, ap, tr fetched
	var wg sync.WaitGroup
	wg.Go(func() { sp = j.fetch(ctx, source.Superpipeline, j.Superpipeline, ref) })
	wg.Go(func() { ap = j.fetch(ctx, source.AgentPod, j.AgentPod, ref) })
	wg.Go(func() { tr = j.fetch(ctx, source.Traces, j.Traces, ref) })
	wg.Wait()

	if sp.status == source.StatusNotFound && ap.status == source.StatusNotFound {
		return RunDocument{}, ErrRunNotFound
	}

	doc := newDocument(ref)
	doc.Sources[string(source.Superpipeline)] = sp.status
	doc.Sources[string(source.AgentPod)] = ap.status
	doc.Sources[string(source.Traces)] = tr.status

	var run *source.RunFragment
	if sp.status == source.StatusOK && sp.frag.Run != nil {
		run = sp.frag.Run
		applyRun(&doc, run)
	}
	if ap.status == source.StatusOK && ap.frag.Ledger != nil {
		applyLedger(&doc, ap.frag.Ledger, run != nil)
	}
	if tr.status == source.StatusOK && tr.frag.Traces != nil {
		applyTraces(&doc, tr.frag.Traces)
	}

	phase2 := ref.WithTraceIDs(doc.Trace.TraceIDs)
	subjects := []verdicts.SubjectKey{{Kind: verdicts.SubjectRun, Ref: ref.String()}}
	for _, a := range doc.Attempts {
		subjects = append(subjects, verdicts.SubjectKey{Kind: verdicts.SubjectAttempt, Ref: a.ID})
	}
	var gates []source.SPGate
	if run != nil {
		gates = run.Gates
	}

	var lg, er fetched
	var owned []verdicts.Verdict
	verdictStatus := source.StatusOK
	var judges map[string]judge
	wg.Go(func() { lg = j.fetch(ctx, source.Logs, j.Logs, phase2) })
	wg.Go(func() { er = j.fetch(ctx, source.Errors, j.Errors, phase2) })
	wg.Go(func() { owned, verdictStatus = j.readVerdicts(ctx, subjects) })
	wg.Go(func() { judges = j.resolveJudges(ctx, gates) })
	wg.Wait()

	doc.Sources[string(source.Logs)] = lg.status
	if lg.status == source.StatusOK && lg.frag.Logs != nil {
		doc.LogCount = KnownCount(lg.frag.Logs.Count)
	}
	doc.Sources[string(source.Errors)] = er.status
	if er.status == source.StatusOK && er.frag.Errors != nil {
		for _, e := range er.frag.Errors.Errors {
			doc.Errors = append(doc.Errors, ErrorItem{Service: orUnknown(e.Service), Message: e.Message, TraceID: e.TraceID, At: fmtTime(e.At)})
		}
	}
	doc.Sources["verdicts"] = verdictStatus
	doc.Verdicts = append(doc.Verdicts, gateViews(ref, gates, judges)...)
	for _, v := range owned {
		doc.Verdicts = append(doc.Verdicts, ownedView(v))
	}
	return doc, nil
}

func applyRun(doc *RunDocument, rf *source.RunFragment) {
	r := rf.Run
	doc.Run.Card = CardView{ID: orUnknown(rf.Card.ID), Title: orUnknown(rf.Card.Title)}
	doc.Run.Stage = orUnknown(r.StageKey)
	doc.Run.AgentRef = orUnknown(r.AgentID)
	doc.Run.Agent = orUnknown(r.AgentPrincipalID)
	doc.Run.State = orUnknown(r.Status)
	if r.Outcome != nil && *r.Outcome != "" {
		doc.Run.State = *r.Outcome
	}
	doc.Run.StartedAt = fmtTime(r.StartedAt)
	doc.Run.EndedAt = fmtTimePtr(r.EndedAt)
	if rf.Usage.Status == "reported" {
		doc.Cost = Cost{Status: "reported", InputTokens: rf.Usage.InputTokens, OutputTokens: rf.Usage.OutputTokens, USD: rf.Usage.CostUSD}
	} else {
		doc.Cost = Cost{Status: "unreported"}
	}
}

func applyLedger(doc *RunDocument, l *source.LedgerFragment, haveRun bool) {
	for _, a := range l.Attempts {
		fp := a.Fingerprint
		doc.Attempts = append(doc.Attempts, Attempt{
			ID: a.ID, Station: orUnknown(a.StationID), State: orUnknown(a.State), SessionID: orUnknown(a.SessionID),
			SeqFrom: a.StartSeq, SeqTo: a.EndSeq, StartedAt: fmtTime(a.StartedAt), EndedAt: fmtTimePtr(a.EndedAt),
			Fingerprint: Fingerprint{Digest: orUnknown(fp.Digest), Harness: orUnknown(fp.Harness),
				HarnessVersion: orUnknown(fp.HarnessVersion), Model: orUnknown(fp.Model), Profile: orUnknown(fp.Profile),
				SkillRelease: orUnknown(fp.SkillRelease), ReportedBy: orUnknown(fp.ReportedBy)},
		})
	}
	if haveRun {
		return
	}
	// superpipeline could not answer (for example, a deleted card); use the ledger.
	if l.CardID != nil {
		doc.Run.Card.ID = orUnknown(*l.CardID)
	}
	if l.Dispatch != nil {
		doc.Run.State = orUnknown(l.Dispatch.Outcome)
	}
	for _, a := range l.Attempts {
		if a.AgentPrincipalID != "" {
			doc.Run.Agent = a.AgentPrincipalID
			break
		}
	}
	var earliest time.Time
	for _, a := range l.Attempts {
		if earliest.IsZero() || a.StartedAt.Before(earliest) {
			earliest = a.StartedAt
		}
	}
	doc.Run.StartedAt = fmtTime(earliest)
}

func applyTraces(doc *RunDocument, tf *source.TracesFragment) {
	doc.Trace.TraceIDs = append([]string{}, tf.TraceIDs...)
	counts := map[string]int{}
	for _, s := range tf.Spans {
		if id := s.Attributes["attempt.id"]; id != "" {
			counts[id]++
		}
	}
	for i := range doc.Attempts {
		doc.Attempts[i].SpanCount = KnownCount(counts[doc.Attempts[i].ID])
	}
	switch {
	case len(tf.Spans) == 0:
		doc.Trace.Status = TraceNone // telemetry missing is not evidence missing
	case len(tf.TraceIDs) == 1 && hasDispatchRoot(tf.Spans):
		doc.Trace.Status, doc.Trace.Sampled = TraceJoined, Flag{V: true, Known: true}
	default:
		doc.Trace.Status, doc.Trace.Sampled = TracePartial, Flag{V: true, Known: true}
	}
}

func hasDispatchRoot(spans []source.Span) bool {
	for _, s := range spans {
		if s.Name == "dispatch" && s.ParentSpanID == "" {
			return true
		}
	}
	return false
}

// judge is a gate decider as resolved for display: a prn_ id and kind, or empty.
type judge struct {
	ID   string
	Kind string
}

// resolveJudges resolves each decided gate's judge: decided_by_principal_id
// if present, else decided_by_hub_sub through the hub's principals route. Any failure leaves
// that judge unresolved, which shows as judge_kind "unknown" and never fails the document.
func (j *Joiner) resolveJudges(ctx context.Context, gates []source.SPGate) map[string]judge {
	out := map[string]judge{}
	cctx, cancel := context.WithTimeout(ctx, j.timeout())
	defer cancel()
	cache := map[string]judge{}
	lookup := func(key string) judge {
		if r, ok := cache[key]; ok {
			return r
		}
		r := judge{}
		if j.Principals != nil {
			if p, err := j.Principals.Lookup(cctx, key); err == nil {
				r = judge{ID: p.ID, Kind: judgeKindOf(p.Kind)}
			}
		}
		cache[key] = r
		return r
	}
	for _, g := range gates {
		switch {
		case g.DecidedByPrincipalID != "":
			r := lookup(g.DecidedByPrincipalID)
			r.ID = g.DecidedByPrincipalID // the producer's mapping stands even if the kind lookup failed
			out[g.ID] = r
		case g.DecidedByHubSub != "":
			out[g.ID] = lookup(g.DecidedByHubSub)
		}
	}
	return out
}

func (j *Joiner) readVerdicts(ctx context.Context, subjects []verdicts.SubjectKey) ([]verdicts.Verdict, source.SourceStatus) {
	if j.Verdicts == nil {
		return nil, source.StatusUnavailable
	}
	cctx, cancel := context.WithTimeout(ctx, j.timeout())
	defer cancel()
	vs, err := j.Verdicts.ListCurrent(cctx, subjects)
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return nil, source.StatusTimeout
		}
		return nil, source.StatusUnavailable
	}
	return vs, source.StatusOK
}

// gateViews shows superpipeline's gates in the verdict shape. They are shown, never
// stored. A pending gate is value {"decision":null}, status "pending".
// A cancelled gate is value {"decision":null}, status "cancelled", At = resolved_at else created_at.
func gateViews(ref source.RunRef, gates []source.SPGate, judges map[string]judge) []VerdictView {
	out := []VerdictView{}
	for _, g := range gates {
		v := VerdictView{ID: g.ID, Kind: "gate", Source: "superpipeline", Subject: ref.String(),
			Standard: "stage:" + g.StageKey, Judge: Unknown, JudgeKind: Unknown, RunID: Unknown}
		if g.RunID != nil && *g.RunID != "" {
			v.RunID = *g.RunID
		}
		resolveJudge := func() {
			if jd := judges[g.ID]; jd.ID != "" {
				v.Judge = jd.ID
				if jd.Kind != "" {
					v.JudgeKind = jd.Kind
				}
			} else if g.DecidedBy != nil && *g.DecidedBy != "" {
				v.Judge = *g.DecidedBy // unmapped superpipeline id: shown, never treated as a principal
			}
		}
		switch {
		case g.Status == "resolved" && g.Decision != nil:
			v.Status = "resolved"
			v.Value, _ = json.Marshal(map[string]string{"decision": *g.Decision})
			resolveJudge()
			v.At = Unknown
			if g.ResolvedAt != nil {
				v.At = fmtTime(*g.ResolvedAt)
			}
		case g.Status == "cancelled":
			// superpipeline cancelled it: no decision, and never shown as pending.
			v.Status = "cancelled"
			v.Value = json.RawMessage(`{"decision":null}`)
			resolveJudge()
			v.At = fmtTime(g.CreatedAt)
			if g.ResolvedAt != nil {
				v.At = fmtTime(*g.ResolvedAt)
			}
		case g.Status == "pending" || g.Status == "resolved": // resolved without a decision is not yet decided
			v.Status = "pending"
			v.Value = json.RawMessage(`{"decision":null}`)
			v.At = fmtTime(g.CreatedAt)
		default: // a status this build does not know: missing data, never pending
			v.Status = Unknown
			v.Value = json.RawMessage(`{"decision":null}`)
			v.At = fmtTime(g.CreatedAt)
		}
		out = append(out, v)
	}
	return out
}

func ownedView(v verdicts.Verdict) VerdictView {
	return VerdictView{ID: v.ID, Kind: v.Kind, Source: "superwitness", Subject: v.SubjectRef, Value: v.Value,
		Judge: v.Judge, JudgeKind: string(v.JudgeKind), Standard: v.Standard, Comment: v.Comment,
		At: fmtTime(v.CreatedAt), Supersedes: v.Supersedes}
}
