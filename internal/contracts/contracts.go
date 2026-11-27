// Package contracts embeds the recorded payloads superwitness reads that no suite contract
// holds (VictoriaTraces' Jaeger API), and the run report schema that sources validate their
// reports against. The suite's crossings (SuperPipeline's and AgentPod's evidence reads, the
// telemetry conventions) are treaty's data, vendored in internal/treaty.
package contracts

import _ "embed"

const (
	TraceHub     = "4bf92f3577b34da6a3ce929d0e0e4736"
	TraceWorkers = "5bf92f3577b34da6a3ce929d0e0e4737"
)

var (
	//go:embed jaeger_trace_hub.json
	JaegerTraceHub []byte
	//go:embed jaeger_trace_workers.json
	JaegerTraceWorkers []byte
	//go:embed run-report.schema.json
	RunReportSchema []byte
)
