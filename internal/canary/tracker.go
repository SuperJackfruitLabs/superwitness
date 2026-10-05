package canary

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

func AppendResult(path string, r Result) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// LoadResults is strict: a corrupt line is an error, never a silently shorter history,
// because a shorter history can drop a proof or hide a regression.
func LoadResults(path string) ([]Result, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	line := 0
	for sc.Scan() {
		line++
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var r Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

type FlowStatus struct {
	Flow        int        `json:"flow"`
	Name        string     `json:"name"`
	Proven      bool       `json:"proven"`
	Proof       *FlowProof `json:"proof,omitempty"`
	Regressed   bool       `json:"regressed"`
	LastFailure string     `json:"last_failure,omitempty"`
}

type Summary struct {
	RunsRecorded int          `json:"runs_recorded"`
	LastRun      string       `json:"last_run,omitempty"`
	LastRunPass  bool         `json:"last_run_pass"`
	Proven       int          `json:"proven"`
	Accepted     bool         `json:"accepted"`
	AcceptedAt   string       `json:"accepted_at,omitempty"`
	Flows        []FlowStatus `json:"flows"`
}

// Summarize applies the acceptance rule. A flow's FIRST proof is kept forever, from any run, nightly
// or manual. A flow whose most recent check failed AFTER its proof is a regression; it
// stays proven. Acceptance is all ten proven, dated by the last flow to be proven.
func Summarize(results []Result) Summary {
	var s Summary
	first := map[int]FlowProof{}
	type seen struct {
		pass        bool
		at          time.Time
		run, detail string
	}
	latest := map[int]seen{}
	gateFail := map[int]seen{} // failing GateResolution state; only a passing resolution clears it
	for _, r := range results {
		for _, p := range r.Proofs {
			if _, ok := first[p.Flow]; !ok {
				first[p.Flow] = p
			}
		}
		if r.Trigger == TriggerAlert || r.Forced {
			continue
		}
		s.RunsRecorded++
		s.LastRun = r.StartedAt.UTC().Format(time.RFC3339)
		s.LastRunPass = r.Pass
		ended := r.EndedAt
		if ended.IsZero() {
			ended = r.StartedAt
		}
		for _, c := range r.Checks {
			n, err := strconv.Atoi(c.ID)
			if err != nil {
				continue
			}
			latest[n] = seen{pass: c.Pass, at: ended, run: r.RunID, detail: c.Detail}
		}
		// A gate a later run saw decided (or cancelled) is flow 6 evidence about the EARLIER
		// run; a failing one is a flow 6 failure, a passing one clears it.
		for _, g := range r.GateResolutions {
			at := g.ObservedAt
			if at.IsZero() {
				at = ended
			}
			if g.Pass {
				delete(gateFail, 6)
			} else {
				gateFail[6] = seen{at: at, run: g.RunID, detail: fmt.Sprintf("gate %s %s (%s) %s", g.GateID, g.Decision, g.JudgeKind, g.Detail)}
			}
		}
	}
	var acceptedAt time.Time
	for n := 1; n <= FlowCount; n++ {
		st := FlowStatus{Flow: n, Name: FlowNames[n]}
		if p, ok := first[n]; ok {
			p := p
			st.Proven, st.Proof = true, &p
			s.Proven++
			if p.At.After(acceptedAt) {
				acceptedAt = p.At
			}
		}
		l, ok := latest[n]
		if g, gok := gateFail[n]; gok && (!ok || l.pass || g.at.After(l.at)) {
			l, ok = g, true
		}
		if ok && !l.pass {
			st.LastFailure = fmt.Sprintf("%s at %s: %s", q(l.run), l.at.UTC().Format(time.RFC3339), l.detail)
			st.Regressed = st.Proven && l.at.After(st.Proof.At)
		}
		s.Flows = append(s.Flows, st)
	}
	s.Accepted = s.Proven == FlowCount
	if s.Accepted {
		s.AcceptedAt = acceptedAt.UTC().Format(time.RFC3339)
	}
	return s
}

var flowHints = map[int]string{
	6:  "approve any canary card's gate (supi gates <board>, supi approve <board> <gate>); the next run records it",
	10: "force a failure through your alerting wrapper (superwitness canary run --force-fail); the wrapper posts to ntfy, then calls canary record-alert",
}

func FormatSummary(s Summary) string {
	var b strings.Builder
	b.WriteString("superwitness canary: acceptance (ten flows, each proven once)\n")
	last := "none"
	if s.LastRun != "" {
		last = s.LastRun + " FAIL"
		if s.LastRunPass {
			last = s.LastRun + " PASS"
		}
	}
	fmt.Fprintf(&b, "runs recorded: %d   last run: %s\n\n", s.RunsRecorded, last)
	for _, f := range s.Flows {
		state := "not yet"
		switch {
		case f.Regressed:
			state = "PROVEN, REGRESSED"
		case f.Proven:
			state = "PROVEN"
		}
		fmt.Fprintf(&b, "%3d  %-18s %s\n", f.Flow, state, f.Name)
		if f.Proof != nil {
			fmt.Fprintf(&b, "       proof: %s at %s\n       output: %s\n       query: %s\n",
				f.Proof.RunRef, f.Proof.At.UTC().Format(time.RFC3339), f.Proof.Output, f.Proof.Query)
		}
		if f.LastFailure != "" && (f.Regressed || !f.Proven) {
			fmt.Fprintf(&b, "       last failure: %s\n", f.LastFailure)
		}
		if hint, ok := flowHints[f.Flow]; ok && !f.Proven {
			fmt.Fprintf(&b, "       next: %s\n", hint)
		}
	}
	b.WriteString("\n")
	if s.Accepted {
		fmt.Fprintf(&b, "ACCEPTED at %s (all %d flows proven)\n", s.AcceptedAt, FlowCount)
	} else {
		fmt.Fprintf(&b, "NOT YET ACCEPTED: %d/%d flows proven\n", s.Proven, FlowCount)
	}
	return b.String()
}
