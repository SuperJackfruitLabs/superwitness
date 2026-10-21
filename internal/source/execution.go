// Execution evidence is a rebuildable metadata projection owned by Salaam/AgentPod.
package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const ExecutionSchemaV1 = "salaam-execution-evidence/v1"

type ExecutionBinding struct {
	Version        int                 `json:"version"`
	OrganizationID string              `json:"organization_id"`
	PrincipalID    string              `json:"principal_id"`
	NodeID         string              `json:"node_id"`
	StationID      string              `json:"station_id"`
	StationKey     string              `json:"station_key"`
	OccupancyEpoch int64               `json:"occupancy_epoch"`
	HubSessionID   string              `json:"hub_session_id"`
	Run            ExecutionRunBinding `json:"run"`
}

type ExecutionRunBinding struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Epoch  int64  `json:"epoch"`
}

type ExecutionTask struct {
	BoardID      string `json:"board_id"`
	CardID       string `json:"card_id"`
	StageKey     string `json:"stage_key"`
	State        string `json:"state"`
	PlanRevision int64  `json:"plan_revision"`
	TaskID       string `json:"task_id"`
	RequestID    string `json:"request_id"`
	SessionID    string `json:"session_id"`
	InputDigest  string `json:"input_digest"`
	PlanDigest   string `json:"plan_digest"`
}

type ExecutionAttempt struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	WorkerEpoch *int64 `json:"worker_epoch"`
	State       string `json:"state"`
}

type ExecutionChild struct {
	ID                     string  `json:"id"`
	ParentSession          string  `json:"parent_session"`
	ParentAttempt          *string `json:"parent_attempt"`
	ChildSession           string  `json:"child_session"`
	ChildAttempt           *string `json:"child_attempt"`
	State                  string  `json:"state"`
	InputDigest            string  `json:"input_digest"`
	ArtifactID             *string `json:"artifact_id"`
	ContributorPrincipalID *string `json:"contributor_principal_id"`
}

type ExecutionToolkitResolutionEvidence struct {
	ResolutionID          string `json:"resolution_id"`
	CallRevision          int64  `json:"call_revision"`
	RequestDigest         string `json:"request_digest"`
	Source                string `json:"source"`
	Outcome               string `json:"outcome"`
	EvidenceDigest        string `json:"evidence_digest"`
	NativeState           string `json:"native_state"`
	NativeReceiptDigest   string `json:"native_receipt_digest"`
	Authority             bool   `json:"authority"`
	ProductEffectVerified bool   `json:"product_effect_verified"`
}
type ExecutionTool struct {
	ResolutionEvidence *ExecutionToolkitResolutionEvidence `json:"resolution_evidence"`
	ID                 string                              `json:"id"`
	SessionID          string                              `json:"session_id"`
	AttemptID          *string                             `json:"attempt_id"`
	ToolCallID         *string                             `json:"tool_call_id"`
	Kind               string                              `json:"kind"`
	State              string                              `json:"state"`
	InputDigest        *string                             `json:"input_digest"`
	Source             string                              `json:"source"`
	TaskCheckID        *string                             `json:"task_check_id"`
	OperationID        *string                             `json:"operation_id"`
	OperationDigest    *string                             `json:"operation_digest"`
	ReceiptDigest      *string                             `json:"receipt_digest"`
	Resolution         *string                             `json:"resolution"`
}

type ExecutionCheck struct {
	ID              string  `json:"id"`
	TaskID          string  `json:"task_id"`
	StepID          *string `json:"step_id"`
	PlanRevision    int64   `json:"plan_revision"`
	WorkspaceDigest string  `json:"workspace_digest"`
	ProfileDigest   string  `json:"profile_digest"`
	EngineID        string  `json:"engine_id"`
	State           string  `json:"state"`
	ArtifactID      *string `json:"artifact_id"`
}

type ExecutionIntegration struct {
	ID            string  `json:"id"`
	ParentSession string  `json:"parent_session"`
	ChildID       string  `json:"child_id"`
	PatchDigest   string  `json:"patch_digest"`
	BeforeDigest  string  `json:"before_digest"`
	AfterArtifact *string `json:"after_artifact"`
	State         string  `json:"state"`
	MutationID    *string `json:"mutation_id"`
}

type ExecutionArtifact struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Digest      string `json:"digest"`
	Bytes       int64  `json:"bytes"`
	Disposition string `json:"disposition"`
}

type ExecutionAmounts struct {
	InputTokens      float64 `json:"input_tokens"`
	OutputTokens     float64 `json:"output_tokens"`
	CacheReadTokens  float64 `json:"cache_read_tokens"`
	CacheWriteTokens float64 `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

type ExecutionObservation struct {
	SourceKey         string            `json:"source_key"`
	SessionID         string            `json:"session_id"`
	AttemptID         *string           `json:"attempt_id"`
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	Status            string            `json:"status"`
	RecordedValues    *ExecutionAmounts `json:"recorded_values"`
	ObservationDigest *string           `json:"observation_digest"`
	DigestVerified    *bool             `json:"digest_verified"`
	TokenProvenance   string            `json:"token_provenance"`
	CostProvenance    string            `json:"cost_provenance"`
}

type ExecutionUsageCoverage struct {
	Observations int64 `json:"observations"`
	Recorded     int64 `json:"recorded"`
	Unknown      int64 `json:"unknown"`
	Estimated    int64 `json:"estimated"`
}

type ExecutionUsage struct {
	Version          int                    `json:"version"`
	RecordedBy       string                 `json:"recorded_by"`
	SessionID        string                 `json:"session_id"`
	IncludesChildren bool                   `json:"includes_children"`
	Status           string                 `json:"status"`
	Coverage         ExecutionUsageCoverage `json:"coverage"`
	RecordedSubtotal *ExecutionAmounts      `json:"recorded_subtotal"`
	CostProvenance   string                 `json:"cost_provenance"`
	Truncated        bool                   `json:"truncated"`
	Observations     []ExecutionObservation `json:"observations"`
}

type ExecutionContributors struct {
	PrincipalIDs []string `json:"principal_ids"`
	Closure      string   `json:"closure"`
}

type ExecutionCoverage struct {
	Truncated   []string `json:"truncated"`
	Unavailable []string `json:"unavailable"`
}

type ExecutionFingerprint struct {
	SessionID              string  `json:"session_id"`
	WorkerEpoch            int64   `json:"worker_epoch"`
	Version                int     `json:"version"`
	Harness                string  `json:"harness"`
	HarnessVersion         string  `json:"harness_version"`
	PiVersion              string  `json:"pi_version"`
	BundleDigest           string  `json:"bundle_digest"`
	PolicyDigest           string  `json:"policy_digest"`
	Provider               string  `json:"provider"`
	Model                  string  `json:"model"`
	ContextDigest          string  `json:"context_digest"`
	ContextAdmissionDigest *string `json:"context_admission_digest"`
	Verification           string  `json:"verification"`
	Digest                 string  `json:"digest"`
}
type ExecutionEvidence struct {
	Fingerprints  []ExecutionFingerprint `json:"fingerprints"`
	SchemaVersion string                 `json:"schema_version"`
	Source        string                 `json:"source"`
	AsOf          string                 `json:"as_of"`
	Binding       ExecutionBinding       `json:"binding"`
	Task          ExecutionTask          `json:"task"`
	Attempts      []ExecutionAttempt     `json:"attempts"`
	Children      []ExecutionChild       `json:"children"`
	Tools         []ExecutionTool        `json:"tools"`
	Checks        []ExecutionCheck       `json:"checks"`
	Integrations  []ExecutionIntegration `json:"integrations"`
	Artifacts     []ExecutionArtifact    `json:"artifacts"`
	Usage         ExecutionUsage         `json:"usage"`
	Contributors  ExecutionContributors  `json:"contributors"`
	Coverage      ExecutionCoverage      `json:"coverage"`
}

// ExecutionEnvelope is the explicitly negotiated API; Fragment.Version remains as_of.
type ExecutionEnvelope struct {
	SchemaVersion  string              `json:"schema_version"`
	ExternalSource string              `json:"external_source"`
	ExternalRunID  string              `json:"external_run_id"`
	BoardID        *string             `json:"board_id"`
	CardID         *string             `json:"card_id"`
	Executions     []ExecutionEvidence `json:"executions"`
	AsOf           string              `json:"as_of"`
}

// strictShape requires every contract field, including nullable fields. Go's zero
// values must never silently turn missing evidence into a known zero or false.
func strictShape(raw json.RawMessage, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return strictShape(raw, t.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("nonnullable evidence field")
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int64, reflect.Float64:
		var n float64
		if json.Unmarshal(raw, &n) != nil || n < 0 || n > 9007199254740991 {
			return fmt.Errorf("invalid evidence number")
		}
	case reflect.String:
		var v string
		if json.Unmarshal(raw, &v) != nil || len(v) == 0 || len(v) > 256 || hasEvidenceControl(v) {
			return fmt.Errorf("invalid evidence string")
		}

	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		if len(fields) != t.NumField() {
			return fmt.Errorf("missing or unknown evidence fields")
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			v, ok := fields[f.Tag.Get("json")]
			if !ok {
				return fmt.Errorf("missing %s", f.Tag.Get("json"))
			}
			if err := scalarField(v, t.Name(), f.Tag.Get("json")); err != nil {
				return err
			}
			if err := strictShape(v, f.Type); err != nil {
				return fmt.Errorf("%s: %w", f.Tag.Get("json"), err)
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		if len(values) > 256 {
			return fmt.Errorf("evidence collection exceeds limit")
		}
		for _, v := range values {
			if err := strictShape(v, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func scalarField(raw json.RawMessage, typ, field string) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	bad := false
	switch {
	case strings.HasSuffix(field, "digest") || field == "digest":
		bad = !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v)
	case field == "organization_id":
		bad = !regexp.MustCompile(`^org_[a-f0-9]{20}$`).MatchString(v)
	case field == "station_key":
		bad = !regexp.MustCompile(`^salaam:[a-f0-9]{16}$`).MatchString(v)
	case field == "id" && typ == "ExecutionTool":
		bad = !evidenceUUID.MatchString(v) && !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v)
	case field == "id" && oneOf(typ, "ExecutionAttempt", "ExecutionChild", "ExecutionCheck", "ExecutionArtifact", "ExecutionIntegration"):
		bad = !evidenceUUID.MatchString(v)
	case oneOf(field, "task_id", "request_id", "session_id", "parent_session", "child_session", "parent_attempt", "child_attempt", "attempt_id", "artifact_id", "after_artifact", "child_id", "mutation_id", "operation_id", "task_check_id") && typ != "ExecutionBinding":
		bad = !evidenceUUID.MatchString(v)
	}
	if bad {
		return fmt.Errorf("invalid %s.%s", typ, field)
	}
	return nil
}

func (e *ExecutionEnvelope) UnmarshalJSON(raw []byte) error {
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	type plain ExecutionEnvelope
	var v plain
	if err := strictShape(raw, reflect.TypeOf(v)); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	*e = ExecutionEnvelope(v)
	return e.Validate()
}

var evidencePrincipal = regexp.MustCompile(`^prn_[a-f0-9]{20}$`)
var evidenceUUID = regexp.MustCompile(`^(?:[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[1-8][a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$`)

func (e *ExecutionEnvelope) Validate() error {
	bad := func() error { return fmt.Errorf("invalid execution evidence or conflicting join") }
	if e.SchemaVersion != "agentpod-execution-evidence/v1" || e.ExternalSource != "superpipeline" || e.ExternalRunID == "" {
		return bad()
	}
	if _, err := time.Parse(time.RFC3339Nano, e.AsOf); err != nil {
		return bad()
	}
	sessions := map[string]bool{}
	for _, x := range e.Executions {
		if x.SchemaVersion != ExecutionSchemaV1 || x.Source != "salaam" || x.Binding.Version != 1 || x.Binding.Run.Source != e.ExternalSource || x.Binding.Run.ID != e.ExternalRunID || x.Binding.Run.Epoch < 1 || x.Binding.OccupancyEpoch < 1 || !evidencePrincipal.MatchString(x.Binding.PrincipalID) || !evidenceUUID.MatchString(x.Task.TaskID) || !evidenceUUID.MatchString(x.Task.SessionID) || sessions[x.Task.SessionID] {
			return bad()
		}
		sessions[x.Task.SessionID] = true
		if _, err := time.Parse(time.RFC3339Nano, x.AsOf); err != nil {
			return bad()
		}
		if e.BoardID == nil || e.CardID == nil || x.Task.BoardID != *e.BoardID || x.Task.CardID != *e.CardID {
			return bad()
		}
		if err := x.validateJoins(); err != nil {
			return err
		}
	}
	return nil
}

func (x ExecutionEvidence) validateJoins() error {
	bad := func() error { return fmt.Errorf("invalid execution evidence join") }
	sessions := map[string]bool{x.Task.SessionID: true}
	attempts := map[string]string{}
	children := map[string]bool{}
	artifacts := map[string]bool{}
	ids := map[string]bool{}
	add := func(id string) bool {
		if id == "" || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, c := range x.Children {
		if !add("child:"+c.ID) || c.ParentSession != x.Task.SessionID || !evidenceUUID.MatchString(c.ChildSession) {
			return bad()
		}
		sessions[c.ChildSession] = true
		children[c.ID] = true
	}
	fingerprints := map[string]bool{}
	for _, fp := range x.Fingerprints {
		key := fmt.Sprintf("%s:%d", fp.SessionID, fp.WorkerEpoch)
		if fingerprints[key] {
			return bad()
		}
		fingerprints[key] = true
		if !sessions[fp.SessionID] || fp.Version != 1 || fp.Harness != "salaam" || fp.Verification != "pinned-readiness-handshake" || fp.Digest != readinessDigest(fp) {
			return bad()
		}
	}
	for _, a := range x.Attempts {
		if !add("attempt:"+a.ID) || !sessions[a.SessionID] {
			return bad()
		}
		attempts[a.ID] = a.SessionID
	}
	linked := func(session string, attempt *string) bool {
		return sessions[session] && (attempt == nil || attempts[*attempt] == session)
	}
	for _, a := range x.Artifacts {
		if !add("artifact:"+a.ID) || !sessions[a.SessionID] || a.Bytes < 0 || !oneOf(a.Disposition, "observed", "proposed", "produced", "integrated") {
			return bad()
		}
		artifacts[a.ID] = true
	}
	artifact := func(id *string) bool { return id == nil || artifacts[*id] }
	for _, c := range x.Children {
		if !linked(c.ParentSession, c.ParentAttempt) || !linked(c.ChildSession, c.ChildAttempt) || !artifact(c.ArtifactID) {
			return bad()
		}
	}
	for _, t := range x.Tools {
		if r := t.ResolutionEvidence; r != nil {
			if t.Source != "managed-toolkit" || r.Source != "operator-attestation" || !oneOf(r.Outcome, "succeeded", "failed_definite") || !oneOf(r.NativeState, "outcome_acknowledged", "returned", "denied", "cancelled") || r.Authority || r.ProductEffectVerified || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(r.ResolutionID) {
				return bad()
			}
		}
		if !add("tool:"+t.Source+":"+t.ID) || !linked(t.SessionID, t.AttemptID) || !oneOf(t.Source, "operation", "managed-http", "managed-toolkit", "station") {
			return bad()
		}
	}
	for _, c := range x.Checks {
		if !add("check:"+c.ID) || c.TaskID != x.Task.TaskID || !artifact(c.ArtifactID) {
			return bad()
		}
	}
	for _, t := range x.Tools {
		if t.OperationID != nil {
			found := false
			for _, op := range x.Tools {
				if op.ID == *t.OperationID && op.Source == "operation" && op.SessionID == t.SessionID {
					found = true
				}
			}
			if !found {
				return bad()
			}
		}
	}
	for _, t := range x.Tools {
		if t.TaskCheckID != nil {
			found := false
			for _, c := range x.Checks {
				if c.ID == *t.TaskCheckID {
					found = true
				}
			}
			if !found {
				return bad()
			}
		}
	}
	for _, i := range x.Integrations {
		if !add("integration:"+i.ID) || i.ParentSession != x.Task.SessionID || !children[i.ChildID] || !artifact(i.AfterArtifact) {
			return bad()
		}
	}
	for _, name := range x.Coverage.Truncated {
		if !oneOf(name, "attempts", "children", "tools", "checks", "integrations", "artifacts", "fingerprints") {
			return bad()
		}
	}
	for _, name := range x.Coverage.Unavailable {
		if !oneOf(name, "attempts", "children", "tools", "checks", "integrations", "artifacts", "usage", "contributors", "fingerprints") {
			return bad()
		}
	}
	if !oneOf(x.Usage.CostProvenance, "ledger-recorded-not-provider-bill", "unknown") {
		return bad()
	}
	if x.Usage.SessionID != x.Task.SessionID || x.Usage.Version != 1 || x.Usage.RecordedBy != "salaam-usage-ledger" || !x.Usage.IncludesChildren || !oneOf(x.Usage.Status, "known", "estimated", "unknown", "unreported", "partial") || !oneOf(x.Contributors.Closure, "complete", "incomplete", "unknown") {
		return bad()
	}
	keys := map[string]bool{}
	for _, o := range x.Usage.Observations {
		if keys[o.SourceKey] || (o.DigestVerified != nil && !*o.DigestVerified) || !oneOf(o.TokenProvenance, "ledger-recorded", "unknown") || !oneOf(o.CostProvenance, "ledger-recorded-not-provider-bill", "unknown") {
			return bad()
		}
		keys[o.SourceKey] = true
		if !linked(o.SessionID, o.AttemptID) || !oneOf(o.Status, "known", "estimated", "unknown") || (o.Status == "unknown" && o.RecordedValues != nil) {
			return bad()
		}
	}
	for _, p := range x.Contributors.PrincipalIDs {
		if p == "" {
			return bad()
		}
	}
	return nil
}
func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ContributorClosure includes the executor and child contributors. Missing identity
// or coverage fails closed for the existing nonhuman separation-of-duty rule.
func (e ExecutionEnvelope) ContributorClosure() ([]string, bool) {
	ids := []string{}
	complete := len(e.Executions) > 0
	for _, x := range e.Executions {
		ids = append(ids, x.Binding.PrincipalID)
		ids = append(ids, x.Contributors.PrincipalIDs...)
		for _, p := range x.Contributors.PrincipalIDs {
			if !evidencePrincipal.MatchString(p) {
				complete = false
			}
		}
		if x.Contributors.Closure != "complete" || containsAny(x.Coverage.Truncated, "attempts", "children") || containsAny(x.Coverage.Unavailable, "contributors", "attempts", "children") {
			complete = false
		}
		for _, c := range x.Children {
			if c.ContributorPrincipalID == nil || !evidencePrincipal.MatchString(*c.ContributorPrincipalID) {
				complete = false
			} else {
				ids = append(ids, *c.ContributorPrincipalID)
			}
		}
	}
	return ids, complete
}

// ContainsReference accepts receipts/artifacts only from this joined snapshot.
func (e ExecutionEnvelope) ContainsReference(kind, id string) bool {
	return e.ContainsTypedReference(kind, id, "")
}
func (e ExecutionEnvelope) ContainsTypedReference(kind, id, receiptSource string) bool {
	for _, x := range e.Executions {
		switch kind {
		case "tool_receipt":
			for _, t := range x.Tools {
				if t.ID == id && t.ReceiptDigest != nil && t.Source == receiptSource {
					return true
				}
			}
		case "artifact":
			for _, a := range x.Artifacts {
				if a.ID == id {
					return true
				}
			}
		}
	}
	return false
}

// Duplicate object keys could otherwise hide a conflicting join from json.Unmarshal.
func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				k := key.(string)
				if seen[k] {
					return fmt.Errorf("duplicate evidence field %s", k)
				}
				seen[k] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid JSON")
		}
		_, err = d.Token()
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func containsAny(values []string, candidates ...string) bool {
	for _, v := range values {
		if oneOf(v, candidates...) {
			return true
		}
	}
	return false
}

// MatchesRun prevents foreign card evidence from crossing the work-plane join.
func (e ExecutionEnvelope) MatchesRun(run SPRun) bool {
	if e.ExternalSource != "superpipeline" || e.ExternalRunID != run.ID || e.BoardID == nil || *e.BoardID != run.BoardID || e.CardID == nil || *e.CardID != run.CardID {
		return false
	}
	return true
}

func hasEvidenceControl(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
