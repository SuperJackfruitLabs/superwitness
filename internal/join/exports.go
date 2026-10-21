package join

import (
	"context"
	"sync"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type ExportSection struct {
	ReferenceID string                         `json:"reference_id"`
	ExportID    string                         `json:"export_id"`
	Status      source.SourceStatus            `json:"status"`
	Receipt     *source.ExecutionExportReceipt `json:"receipt"`
}

func (j *Joiner) readExports(ctx context.Context, work *source.WorkEvidence) []ExportSection {
	ctx, cancel := context.WithTimeout(ctx, j.timeout())
	defer cancel()
	sections := []ExportSection{}
	if work == nil {
		return sections
	}
	resolver, ok := j.AgentPod.(source.ExportResolver)
	for _, ref := range work.Provenance.References {
		if ref.ExecutionExport != nil {
			sections = append(sections, ExportSection{ReferenceID: ref.ID, ExportID: ref.ExecutionExport.ExportID, Status: source.StatusUnavailable})
		}
	}
	if !ok {
		return sections
	}
	// Keep evidence reads bounded even when references were accumulated over a run.
	limit := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range sections {
		wg.Go(func() {
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-limit }()
			cctx, cancel := context.WithTimeout(ctx, j.timeout())
			defer cancel()
			receipt, status := resolver.FetchExport(cctx, sections[i].ExportID)
			if status != source.StatusOK {
				sections[i].Status = status
				return
			}
			var reference *source.WorkReference
			for _, r := range work.Provenance.References {
				if r.ID == sections[i].ReferenceID {
					reference = &r
					break
				}
			}
			run := work.Pipeline.Run
			if reference == nil || receipt.ExportID != sections[i].ExportID || receipt.ExternalSource != "superpipeline" || receipt.ExternalRunID != run.ID || receipt.BoardID != run.BoardID || receipt.CardID != run.CardID || receipt.StageKey != run.StageKey || receipt.RunEpoch != work.Provenance.LeaseEpoch || receipt.ManifestSHA256 != reference.ExecutionExport.ManifestSHA256 {
				return
			}
			sections[i].Status = source.StatusOK
			sections[i].Receipt = &receipt
		})
	}
	wg.Wait()
	return sections
}
