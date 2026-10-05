package docsclaims

import "testing"

// TestMatcherFlagsInternalLines proves each forbidden-pattern family fires on
// a representative internal line and stays quiet on clean ones.
func TestMatcherFlagsInternalLines(t *testing.T) {
	flag := map[string]string{
		"tailnet-ip":      "listen on 100.64.12.3:8080",
		"ts-net":          "https://box.tail1234.ts.net/",
		"ruling":          "per ruling R4 we skip it",
		"ruling-n":        "Ruling 7 applies",
		"task-n":          "see Task 12 in the plan",
		"workstream":      "owned by ws3",
		"milestone":       "ships in milestone 2",
		"spec-section":    "spec §4.1 says so",
		"section-sign":    "see §7",
		"plan-index":      "listed in the plan index",
		"contract-c":      "contract C4 route",
		"per-ruling-id":   "TestContractGateViewsReadPerR4",
		"real-id":         "owner prn_01HZX3K9Q4T7M2V8B5N6C1D0EF",
		"index-c":         "listed at index C3 of the plan",
		"gap-g":           "closes gap G2",
		"review-focus":    "Review focus: the scanner",
		"review-focus-lc": "the review Focus list",
	}
	for name, line := range flag {
		if len(findInternalRefs(line)) == 0 {
			t.Errorf("%s: expected %q to be flagged", name, line)
		}
	}
	clean := []string{
		"const [x, setX] = useState(0)",
		"the infrastructure layer",
		"https://hub.agentpod.dev/v1",
		"install to /etc/superwitness/env",
		"owner prn_grader01 and brd_01 and svc_fake",
		"connect over your tailnet address",
		"Tailnet-first deployment",
		"a task list, task 1 of many", // lowercase "task"
		"PerRequest handler",
	}
	for _, line := range clean {
		if got := findInternalRefs(line); len(got) != 0 {
			t.Errorf("clean line %q flagged: %v", line, got)
		}
	}
}

// TestPrivateNameDigestMatch proves the digest check with a neutral sample
// name, so this file does not spell out the real private names.
func TestPrivateNameDigestMatch(t *testing.T) {
	digests := map[string]bool{tokenDigest("zebrahost"): true}
	for _, line := range []string{
		"ssh zebrahost to restart it",
		"Zebrahost runs the hub",
		"deployed on ZEBRAHOST today",
		"zebrahost/docs/plan.md",
	} {
		got := scanLine(line, digests)
		if len(got) != 1 || got[0].pattern != privateNamePattern {
			t.Errorf("%q: want one %s finding, got %v", line, privateNamePattern, got)
		}
	}
	for _, line := range []string{
		"the zebrahosting layer",
		"subzebrahost box",
		"const [x, setX] = useState(0)",
		"the infrastructure layer",
		"https://hub.agentpod.dev/v1",
		"install to /etc/superwitness/env",
	} {
		if got := scanLine(line, digests); len(got) != 0 {
			t.Errorf("clean line %q flagged: %v", line, got)
		}
	}
	if len(privateNameDigests) != 7 {
		t.Errorf("privateNameDigests has %d entries, want 7", len(privateNameDigests))
	}
}
