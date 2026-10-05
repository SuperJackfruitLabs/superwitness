package canary

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// LogsClient scans every log line in a window. LogsQL word filters apply only to _msg,
// so the canary fetches the whole window and searches every field itself, streaming.
type LogsClient struct {
	BaseURL   string
	TokenFile string
	HTTP      *http.Client
}

func (c LogsClient) Scan(ctx context.Context, start, end time.Time, needles map[string]string) (ScanResult, error) {
	res := newScanResult()
	query := fmt.Sprintf("_time:[%s, %s]", start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
	form := url.Values{"query": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/select/logsql/query",
		strings.NewReader(form.Encode()))
	if err != nil {
		return res, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := setBearer(req, c.TokenFile); err != nil {
		return res, err
	}
	resp, err := orDefault(c.HTTP).Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("/select/logsql/query: HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		res.Scanned++
		for label, n := range needles {
			if n != "" && bytes.Contains(line, []byte(n)) {
				res.hit(label, redact(logLocation(line, n), needles))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("reading the logs stream: %w", err)
	}
	return res, nil
}

// logLocation names the stream, time and fields holding the needle, never the needle.
func logLocation(line []byte, needle string) string {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return "an unparseable line"
	}
	var fields []string
	for k, v := range m {
		if s, ok := v.(string); ok && strings.Contains(s, needle) {
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)
	return fmt.Sprintf("stream=%v time=%v fields=%s", m["_stream"], m["_time"], strings.Join(fields, ","))
}
