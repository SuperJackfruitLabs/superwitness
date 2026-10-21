package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// ReadinessDigest mirrors Salaam protocol.canonical over the flat fingerprint
// record, excluding digest. Session and worker epoch are therefore digest-bound.
func readinessDigest(fp ExecutionFingerprint) string {
	b, _ := json.Marshal(fp)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(b, &fields)
	delete(fields, "digest")
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var canonical bytes.Buffer
	canonical.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			canonical.WriteByte(',')
		}
		writeJSONString(&canonical, k)
		canonical.WriteByte(':')
		var value string
		if json.Unmarshal(fields[k], &value) == nil && string(fields[k]) != "null" {
			writeJSONString(&canonical, value)
		} else {
			canonical.Write(fields[k])
		}
	}
	canonical.WriteByte('}')
	sum := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(sum[:])
}

// JSON.stringify leaves U+2028/U+2029 and HTML characters unescaped. Go's JSON
// encoder differs there; spell JSON strings explicitly to preserve source hashes.
func writeJSONString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
