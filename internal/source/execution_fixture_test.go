package source

import "testing"

func TestExecutionRejectsDuplicateFields(t *testing.T) {
	raw := []byte(`{"a":1,"a":2}`)
	if uniqueJSON(raw) == nil {
		t.Fatal("duplicate join key accepted")
	}
}
