package join

import (
	"context"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type exportSource struct{ receipt source.ExecutionExportReceipt }

func (e exportSource) Name() source.Name { return source.AgentPod }
func (e exportSource) Fetch(context.Context, source.RunRef) (source.Fragment, source.SourceStatus) {
	return source.Fragment{}, source.StatusUnavailable
}
func (e exportSource) FetchExport(context.Context, string) (source.ExecutionExportReceipt, source.SourceStatus) {
	return e.receipt, source.StatusOK
}
func TestExportReceiptMustMatchOriginalRunHint(t *testing.T) {
	r := source.ExecutionExportReceipt{ExportID: "export_1", ExternalSource: "superpipeline", ExternalRunID: "run_01", BoardID: "brd_01", CardID: "card_01", StageKey: "draft", RunEpoch: 1, ManifestSHA256: "digest_1"}
	work := &source.WorkEvidence{Pipeline: source.RunFragment{Run: source.SPRun{ID: "run_01", BoardID: "brd_01", CardID: "card_01", StageKey: "draft"}}, Provenance: source.WorkProvenance{LeaseEpoch: 1, References: []source.WorkReference{{ID: "ref_1", ExecutionExport: &source.WorkExecutionExport{ExportID: "export_1", ManifestSHA256: "digest_1"}}}}}
	j := Joiner{AgentPod: exportSource{r}}
	joined := j.readExports(context.Background(), work)
	if len(joined) != 1 || joined[0].Status != source.StatusOK || joined[0].Receipt == nil {
		t.Fatalf("%+v", joined)
	}
	for _, mutate := range []func(*source.ExecutionExportReceipt){
		func(r *source.ExecutionExportReceipt) { r.RunEpoch++ },
		func(r *source.ExecutionExportReceipt) { r.ManifestSHA256 = "other" },
		func(r *source.ExecutionExportReceipt) { r.ExternalRunID = "other" },
		func(r *source.ExecutionExportReceipt) { r.CardID = "other" },
		func(r *source.ExecutionExportReceipt) { r.StageKey = "other" },
	} {
		foreign := r
		mutate(&foreign)
		j.AgentPod = exportSource{foreign}
		joined = j.readExports(context.Background(), work)
		if joined[0].Status == source.StatusOK || joined[0].Receipt != nil {
			t.Fatal("foreign receipt crossed run binding")
		}
	}
}
