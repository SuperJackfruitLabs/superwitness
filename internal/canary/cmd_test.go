package canary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func runCmdWith(t *testing.T, d Deps, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	build := func(context.Context, Config, []Result) (Deps, error) { return d, nil }
	code := runCmd(context.Background(), args, envFrom(env), &out, &errb, build)
	return code, out.String(), errb.String()
}

func TestRunCmdUsage(t *testing.T) {
	d, _ := goodDeps()
	if code, _, _ := runCmdWith(t, d, fullEnv(), "--trigger", "nightly"); code != 2 {
		t.Errorf("no --results: code %d", code)
	}
	results := filepath.Join(t.TempDir(), "r.jsonl")
	if code, _, _ := runCmdWith(t, d, fullEnv(), "--trigger", "hourly", "--results", results); code != 2 {
		t.Errorf("bad trigger: code %d", code)
	}
}

func TestRunCmdRecordsASetupFailure(t *testing.T) {
	d, _ := goodDeps()
	dir := t.TempDir()
	results, last := filepath.Join(dir, "r.jsonl"), filepath.Join(dir, "last.json")
	env := fullEnv()
	delete(env, "SWC_BOARD_ID")
	code, out, _ := runCmdWith(t, d, env, "--trigger", "nightly", "--results", results, "--result-out", last)
	if code != 1 || !strings.Contains(out, "canary FAIL") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	var r Result
	b, _ := os.ReadFile(last)
	if err := json.Unmarshal(b, &r); err != nil || !strings.Contains(r.Error, "setup: missing environment: SWC_BOARD_ID") {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	if rs, _ := LoadResults(results); len(rs) != 1 {
		t.Fatalf("tracker has %d lines", len(rs))
	}
}

func TestRunCmdPassesAndRecordsProofs(t *testing.T) {
	d, _ := goodDeps()
	results := filepath.Join(t.TempDir(), "r.jsonl")
	env := fullEnv()
	env["SWC_BOARD_ID"] = "brd_c" // goodDeps' fake document is for board brd_c
	code, out, _ := runCmdWith(t, d, env, "--trigger", "manual", "--results", results)
	if code != 0 || out != "canary PASS run=run_1 1=ok 2=ok 3=ok 4=ok 5=ok 6=ok 7=ok 8=ok 9=ok proofs=8\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	rs, _ := LoadResults(results)
	if s := Summarize(rs); s.Proven != 8 {
		t.Fatalf("proven = %d", s.Proven)
	}
}

func TestRunCmdForceFailDispatchesNothing(t *testing.T) {
	results := filepath.Join(t.TempDir(), "r.jsonl")
	var out, errb bytes.Buffer
	build := func(context.Context, Config, []Result) (Deps, error) {
		t.Fatal("a forced failure must not build clients or dispatch a card")
		return Deps{}, nil
	}
	code := runCmd(context.Background(), []string{"--trigger", "manual", "--force-fail", "--results", results},
		envFrom(fullEnv()), &out, &errb, build)
	rs, _ := LoadResults(results)
	if code != 1 || len(rs) != 1 || !rs[0].Forced || rs[0].Pass || !strings.Contains(rs[0].Error, "flow 10") {
		t.Fatalf("code=%d rs=%+v", code, rs)
	}
}

func TestRecordAlertAfterAForcedFailure(t *testing.T) {
	results := filepath.Join(t.TempDir(), "r.jsonl")
	var out, errb bytes.Buffer
	_ = runCmd(context.Background(), []string{"--trigger", "manual", "--force-fail", "--results", results},
		envFrom(fullEnv()), &out, &errb, nil)
	code := Main(context.Background(), []string{"record-alert", "--results", results, "--ntfy-id", "m1",
		"--query", "POST /superwitness-canary (accepted as m1); GET /superwitness-canary/json?poll=1&since=15m (found m1)"}, envFrom(nil), &out, &errb)
	rs, _ := LoadResults(results)
	if code != 0 || len(rs) != 2 || rs[1].Trigger != TriggerAlert || rs[1].Proofs[0].Flow != 10 ||
		!strings.Contains(rs[1].Proofs[0].Output, "m1") || !strings.Contains(rs[1].Proofs[0].Query, "poll=1") {
		t.Fatalf("code=%d stderr=%s rs=%+v", code, errb.String(), rs)
	}
}

func TestRecordAlertRefusesWithoutAForcedFailure(t *testing.T) {
	results := filepath.Join(t.TempDir(), "r.jsonl")
	_ = AppendResult(results, passingRun(day(1), "run_1", TriggerNightly))
	var out, errb bytes.Buffer
	code := Main(context.Background(), []string{"record-alert", "--results", results, "--ntfy-id", "m1", "--query", "q"},
		envFrom(nil), &out, &errb)
	rs, _ := LoadResults(results)
	if code != 2 || len(rs) != 1 {
		t.Fatalf("code=%d lines=%d", code, len(rs))
	}
}

func TestRunCmdRefusesACorruptTracker(t *testing.T) {
	d, _ := goodDeps()
	results := filepath.Join(t.TempDir(), "r.jsonl")
	_ = os.WriteFile(results, []byte("{broken\n"), 0o640)
	if code, _, errs := runCmdWith(t, d, fullEnv(), "--trigger", "nightly", "--results", results); code != 2 || !strings.Contains(errs, "line 1") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestRunCmdNeverPrintsTheMarker(t *testing.T) {
	d, f := goodDeps()
	f.traces.traceErr = fmt.Errorf("span echoed %s", zeroPlain)
	f.logs.scan.Hits[LabelSecretMarker] = 1
	f.logs.scan.Where[LabelSecretMarker] = "stream={x} fields=" + zeroMarker
	dir := t.TempDir()
	results, last := filepath.Join(dir, "r.jsonl"), filepath.Join(dir, "last.json")
	env := fullEnv()
	env["SWC_BOARD_ID"] = "brd_c" // goodDeps' fake document is for board brd_c
	_, out, errs := runCmdWith(t, d, env, "--trigger", "manual", "--results", results, "--result-out", last)
	tracker, _ := os.ReadFile(results)
	lastb, _ := os.ReadFile(last)
	// Non-vacuity: the leak was seen (check 9 failed) and redaction, not absence, hid it.
	if !strings.Contains(out, " 9=FAIL") {
		t.Errorf("check 9 did not fail, so the test proves nothing: %q", out)
	}
	for _, ph := range []string{`\u003c` + LabelSecretMarker + `\u003e`, `\u003c` + LabelPlain + `\u003e`} { // JSON-escaped <label>
		if !strings.Contains(string(tracker), ph) {
			t.Errorf("tracker lacks redaction placeholder %s: %s", ph, tracker)
		}
	}
	for name, s := range map[string]string{"stdout": out, "stderr": errs, "tracker": string(tracker), "result": string(lastb)} {
		if strings.Contains(s, zeroPlain) || strings.Contains(s, zeroMarker) {
			t.Errorf("marker in %s: %s", name, s)
		}
	}
}

func TestStatusCmdJSON(t *testing.T) {
	results := filepath.Join(t.TempDir(), "r.jsonl")
	for _, r := range []Result{passingRun(day(1), "run_1", TriggerNightly), gateProofRun(day(2), "run_2", "run_1"), alertProofLine(day(3))} {
		_ = AppendResult(results, r)
	}
	var out, errb bytes.Buffer
	code := Main(context.Background(), []string{"status", "--results", results, "--json"}, envFrom(nil), &out, &errb)
	var s Summary
	if code != 0 || json.Unmarshal(out.Bytes(), &s) != nil || !s.Accepted || len(s.Flows) != 10 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errb.String())
	}
}

func TestNewDepsLearnsItsPrincipal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"token":%q,"expiresIn":300}`, fakeJWT("prn_canary"))
	}))
	defer srv.Close()
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte("s"), 0o600)
	cfg, _ := LoadConfig(envFrom(fullEnv()))
	cfg.HubURL, cfg.ClientSecretFile = srv.URL, secret
	d, err := NewDeps(context.Background(), cfg, nil)
	if err != nil || d.Principal != "prn_canary" || d.OTLPLogsURL != "http://127.0.0.1:4318/v1/logs" {
		t.Fatalf("deps=%+v err=%v", d, err)
	}
}

func TestRunCmdDeadlineStillRecordsAFailure(t *testing.T) {
	old := processDeadline
	processDeadline = 50 * time.Millisecond
	defer func() { processDeadline = old }()
	dir := t.TempDir()
	results, last := filepath.Join(dir, "r.jsonl"), filepath.Join(dir, "last.json")
	var out, errb bytes.Buffer
	build := func(ctx context.Context, _ Config, _ []Result) (Deps, error) {
		<-ctx.Done() // a hung call that only the process deadline ends
		return Deps{}, ctx.Err()
	}
	code := runCmd(context.Background(), []string{"--trigger", "nightly", "--results", results, "--result-out", last},
		envFrom(fullEnv()), &out, &errb, build)
	rs, _ := LoadResults(results)
	b, _ := os.ReadFile(last)
	if code != 1 || len(rs) != 1 || rs[0].Pass || !strings.Contains(rs[0].Error, "deadline") ||
		!strings.Contains(string(b), "deadline") || !strings.Contains(out.String(), "canary FAIL") {
		t.Fatalf("code=%d rs=%+v last=%s out=%q", code, rs, b, out.String())
	}
}

func TestForceFailResultOutWritesAnEmptyChecksArray(t *testing.T) {
	dir := t.TempDir()
	results, last := filepath.Join(dir, "r.jsonl"), filepath.Join(dir, "last.json")
	var out, errb bytes.Buffer
	code := runCmd(context.Background(), []string{"--trigger", "manual", "--force-fail", "--results", results, "--result-out", last},
		envFrom(fullEnv()), &out, &errb, nil)
	b, _ := os.ReadFile(last)
	if code != 1 || !strings.Contains(string(b), `"checks": []`) || strings.Contains(string(b), `"checks": null`) {
		t.Fatalf("code=%d result=%s", code, b)
	}
	tr, _ := os.ReadFile(results)
	if !strings.Contains(string(tr), `"checks":[]`) {
		t.Fatalf("tracker=%s", tr)
	}
	// Golden: the exact wire shape an alerting wrapper's fixtures mirror (timestamps pinned).
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["started_at"], m["ended_at"] = "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"
	got, _ := json.MarshalIndent(m, "", "  ")
	want, err := os.ReadFile("testdata/forced-result.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Fatalf("golden mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetupFailureResultOutWritesAnEmptyChecksArray(t *testing.T) {
	d, _ := goodDeps()
	dir := t.TempDir()
	results, last := filepath.Join(dir, "r.jsonl"), filepath.Join(dir, "last.json")
	env := fullEnv()
	delete(env, "SWC_BOARD_ID")
	runCmdWith(t, d, env, "--trigger", "nightly", "--results", results, "--result-out", last)
	b, _ := os.ReadFile(last)
	if !strings.Contains(string(b), `"checks": []`) || !strings.Contains(string(b), "setup:") {
		t.Fatalf("result=%s", b)
	}
}

func TestRunCmdFailsWhenTheResultCannotBeRecorded(t *testing.T) {
	d, _ := goodDeps()
	dir := t.TempDir()
	results, last := filepath.Join(dir, "no-such-dir", "r.jsonl"), filepath.Join(dir, "last.json")
	env := fullEnv()
	env["SWC_BOARD_ID"] = "brd_c"
	code, out, errs := runCmdWith(t, d, env, "--trigger", "manual", "--results", results, "--result-out", last)
	b, _ := os.ReadFile(last)
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("result=%s err=%v", b, err)
	}
	if code != 1 || r.Pass || !strings.Contains(r.Error, "recording the result:") ||
		!strings.Contains(out, "canary FAIL") || !strings.Contains(errs, "recording the result") {
		t.Fatalf("code=%d result=%s out=%q stderr=%q", code, b, out, errs)
	}
}

func TestNewDepsUnderThePlaneUsesOneTokenPerAudience(t *testing.T) {
	var auds []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Audience string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		auds = append(auds, b.Audience)
		fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300}`, fakeJWT("prn_canary"))
	}))
	defer srv.Close()
	cred := filepath.Join(t.TempDir(), "cred")
	_ = os.WriteFile(cred, []byte("svc_canary:s3cret\n"), 0o600)
	env := fullEnv()
	delete(env, "SWC_HUB_CLIENT_ID")
	delete(env, "SWC_HUB_CLIENT_SECRET_FILE")
	delete(env, "SW_HUB_URL")
	env["SW_ORG_PLANE_URL"] = srv.URL
	env["SWC_ORG_PLANE_SERVICE_CREDENTIAL_FILE"] = cred
	env["SWC_SUPERWITNESS_AUDIENCE"] = "https://superwitness.example"
	cfg, err := LoadConfig(envFrom(env))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeps(context.Background(), cfg, nil)
	if err != nil || d.Principal != "prn_canary" {
		t.Fatalf("deps=%+v err=%v", d, err)
	}
	if _, err := d.SP.(SuperpipelineClient).Tokens.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://superwitness.example", cfg.SuperpipelineURL}
	if !slices.Equal(auds, want) {
		t.Errorf("audiences = %v, want %v", auds, want)
	}
}

func TestLoadConfigUnderThePlaneNeedsTheSuperwitnessAudience(t *testing.T) {
	env := fullEnv()
	env["SW_ORG_PLANE_URL"] = "https://accounts.example"
	env["SWC_ORG_PLANE_SERVICE_CREDENTIAL_FILE"] = "/etc/superwitness/canary-credential"
	_, err := LoadConfig(envFrom(env))
	if err == nil || !strings.Contains(err.Error(), "SWC_SUPERWITNESS_AUDIENCE") {
		t.Errorf("err = %v", err)
	}
}
