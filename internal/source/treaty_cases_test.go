package source

import (
	"encoding/json"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// examples is AgentPod's evidence corpus for one schema, from the vendored treaty data.
func examples(t *testing.T, schema string) treaty.Cases {
	t.Helper()
	c, err := treaty.Examples(schema)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// response is one operation's 200 cases from a vendored HTTP cases file.
func response(t *testing.T, file, operation string) treaty.Cases {
	t.Helper()
	c, err := treaty.Response(file, operation, "200")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func validCase(t *testing.T, c treaty.Cases, name string) json.RawMessage {
	t.Helper()
	b, err := c.Case(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
