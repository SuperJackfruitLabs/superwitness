package docsclaims

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
)

// The docs site serves the run report schema at its $id; it must be the file the code embeds.
func TestPublishedSchemaIsTheEmbeddedOne(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs-site/public/schemas/run-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(b), bytes.TrimSpace(contracts.RunReportSchema)) {
		t.Error("docs-site/public/schemas/run-report.schema.json differs from internal/contracts/run-report.schema.json; copy it")
	}
}
