// Package contracts embeds the recorded payloads superwitness consumes (superpipeline's
// run evidence route, the hub's evidence and principal routes, and VictoriaTraces' Jaeger
// API). Producers validate the same files in their own CI.
package contracts

import _ "embed"

const (
	TraceHub     = "4bf92f3577b34da6a3ce929d0e0e4736"
	TraceWorkers = "5bf92f3577b34da6a3ce929d0e0e4737"
)

var (
	//go:embed superpipeline_evidence.json
	SuperpipelineEvidence []byte
	//go:embed hub_evidence_run.json
	HubEvidenceRun []byte
	//go:embed hub_evidence_attempt.json
	HubEvidenceAttempt []byte
	//go:embed hub_principal.json
	HubPrincipal []byte
	//go:embed jaeger_trace_hub.json
	JaegerTraceHub []byte
	//go:embed jaeger_trace_workers.json
	JaegerTraceWorkers []byte
)
