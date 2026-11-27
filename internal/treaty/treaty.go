// Package treaty is the treaty data SuperWitness reads: the contracts of the crossings it
// consumes and their cases. Every file under this directory except this one is a copy that
// `treaty sync` (scripts/treaty.sh) writes from the release treaty.json pins, and `treaty verify`
// in CI holds each one byte-identical to it. Nothing under schemas/ or fixtures/ is edited by
// hand: to change one, move the pin and run sync.
package treaty

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

//go:embed schemas fixtures
var files embed.FS

// The vendored files SuperWitness reads, by their path in treaty (and under this directory).
const (
	BoardRead        = "schemas/http/superpipeline/board-read.v1.openapi.json"
	BoardReadCases   = "fixtures/http/superpipeline/board-read.v1.cases.json"
	Evidence         = "schemas/http/agentpod/evidence.v1.openapi.json"
	EvidenceCases    = "fixtures/http/agentpod/evidence.v1.cases.json"
	EvidenceExamples = "fixtures/agentpod-evidence/examples.json"
	Telemetry        = "schemas/telemetry/suite.v1.json"
	TelemetryCases   = "fixtures/telemetry/suite.v1.cases.json"
)

// File is one vendored file's bytes, exactly as treaty publishes them.
func File(path string) ([]byte, error) { return files.ReadFile(path) }

// Cases are a contract's examples: bodies a reader must accept, by name, and bodies it must
// refuse, each with the reason.
type Cases struct {
	Valid   map[string]json.RawMessage `json:"valid"`
	Invalid []Invalid                  `json:"invalid"`
}

// Invalid is a body a reader must refuse and why.
type Invalid struct {
	Why  string          `json:"why"`
	Body json.RawMessage `json:"body"`
}

// Case is one named valid body.
func (c Cases) Case(name string) (json.RawMessage, error) {
	b, ok := c.Valid[name]
	if !ok {
		return nil, fmt.Errorf("treaty: no valid case %q", name)
	}
	return b, nil
}

// Response is the cases of one operation's response with the given status in an HTTP cases
// file (fixtures/http/<owner>/<name>.cases.json).
func Response(casesFile, operationID, status string) (Cases, error) {
	var doc struct {
		Operations map[string]struct {
			Responses map[string]Cases `json:"responses"`
		} `json:"operations"`
	}
	if err := decode(casesFile, &doc); err != nil {
		return Cases{}, err
	}
	c, ok := doc.Operations[operationID].Responses[status]
	if !ok || len(c.Valid) == 0 {
		return Cases{}, fmt.Errorf("treaty: %s has no %s %s cases", casesFile, operationID, status)
	}
	return c, nil
}

// Examples is the cases of schemas/agentpod/<name>.json in AgentPod's evidence corpus.
func Examples(name string) (Cases, error) {
	var doc struct {
		Schemas map[string]Cases `json:"schemas"`
	}
	if err := decode(EvidenceExamples, &doc); err != nil {
		return Cases{}, err
	}
	c, ok := doc.Schemas[name]
	if !ok || len(c.Valid) == 0 {
		return Cases{}, fmt.Errorf("treaty: %s has no %s cases", EvidenceExamples, name)
	}
	return c, nil
}

// Operation is one route of an OpenAPI document.
type Operation struct {
	Method string
	Path   string // the path template, as the document spells it: /v1/boards/{boardId}/…
}

// Operations is every operation of an OpenAPI document, by operationId.
func Operations(openapiFile string) (map[string]Operation, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := decode(openapiFile, &doc); err != nil {
		return nil, err
	}
	ops := map[string]Operation{}
	for path, item := range doc.Paths {
		for method, raw := range item {
			var op struct {
				OperationID string `json:"operationId"`
			}
			if json.Unmarshal(raw, &op) != nil || op.OperationID == "" {
				continue // a path item's parameters or summary, not an operation
			}
			ops[op.OperationID] = Operation{Method: strings.ToUpper(method), Path: path}
		}
	}
	return ops, nil
}

// Conventions are the suite's telemetry conventions (schemas/telemetry/suite.v1.json): every
// attribute key a producer may set and SuperWitness may query, and which keys each span and
// log record carries.
type Conventions struct {
	Attributes map[string]Attribute `json:"attributes"`
	Spans      map[string]Signal    `json:"spans"`
	Logs       map[string]Signal    `json:"logs"`
}

// Attribute is one attribute key's type (string, int, double or boolean), the values a string
// may take when the conventions list them, and the least a number may be.
type Attribute struct {
	Type    string   `json:"type"`
	Enum    []string `json:"enum"`
	Minimum *float64 `json:"minimum"`
}

// Admits reports whether v, a value as JSON decodes it (string, bool, float64 or json.Number) or
// as Coerce returns it (int64), is of the attribute's type, on its list and at least its minimum.
// The conventions' cases hold it to their verdicts.
func (a Attribute) Admits(v any) bool {
	switch a.Type {
	case "string":
		s, ok := v.(string)
		return ok && (len(a.Enum) == 0 || slices.Contains(a.Enum, s))
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "int", "double":
		f, ok := number(v)
		if !ok || (a.Type == "int" && f != math.Trunc(f)) {
			return false
		}
		return a.Minimum == nil || f >= *a.Minimum
	}
	return false
}

// Coerce returns v as the attribute's type when v is text (or a number for a string attribute)
// that reads as one: "3" is 3 for an int, "true" is true for a boolean, 3 is "3" for a string. A
// value that does not read as the type, or an attribute of a type Coerce does not know, comes
// back unchanged: telemetry is best-effort, so a reader shows what the producer sent.
func (a Attribute) Coerce(v any) any {
	return CoerceTo(a.Type, v)
}

// CoerceTo is Coerce for a type named string, int, double or boolean.
func CoerceTo(typ string, v any) any {
	s, isText := v.(string)
	if n, ok := v.(json.Number); ok {
		s, isText = n.String(), true
	}
	switch typ {
	case "string":
		switch x := v.(type) {
		case json.Number:
			return x.String()
		case bool:
			return strconv.FormatBool(x)
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
	case "int":
		if isText {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
	case "double":
		if isText {
			if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
				return f
			}
		}
	case "boolean":
		if _, isNumber := v.(json.Number); isText && !isNumber && (s == "true" || s == "false") {
			return s == "true"
		}
	}
	return v
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// Signal is one span or log record: the service that emits it and the keys it carries.
type Signal struct {
	Service  string   `json:"service"`
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

// TelemetryConventions reads schemas/telemetry/suite.v1.json.
func TelemetryConventions() (Conventions, error) {
	var c Conventions
	if err := decode(Telemetry, &c); err != nil {
		return Conventions{}, err
	}
	if len(c.Attributes) == 0 || len(c.Spans) == 0 {
		return Conventions{}, fmt.Errorf("treaty: %s names no attributes or spans", Telemetry)
	}
	return c, nil
}

// SignalCases are the telemetry cases: attribute sets per span and log record, as a producer
// emits them.
type SignalCases struct {
	Spans map[string]Cases `json:"spans"`
	Logs  map[string]Cases `json:"logs"`
}

// TelemetryExamples reads fixtures/telemetry/suite.v1.cases.json.
func TelemetryExamples() (SignalCases, error) {
	var c SignalCases
	if err := decode(TelemetryCases, &c); err != nil {
		return SignalCases{}, err
	}
	return c, nil
}

func decode(path string, out any) error {
	b, err := files.ReadFile(path)
	if err != nil {
		return fmt.Errorf("treaty: %w", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("treaty: %s: %w", path, err)
	}
	return nil
}
