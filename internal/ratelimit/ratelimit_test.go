package ratelimit

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	l := PerMinute(10, 20)
	l.Now = func() time.Time { return now }
	for i := range 20 {
		if ok, _ := l.Allow("198.51.100.7"); !ok {
			t.Fatalf("request %d of the burst refused", i+1)
		}
	}
	ok, wait := l.Allow("198.51.100.7")
	if ok || wait < 5*time.Second || wait > 6*time.Second {
		t.Errorf("21st: ok=%v wait=%v; want refused for about 6s (10 a minute)", ok, wait)
	}
	if ok, _ := l.Allow("203.0.113.9"); !ok {
		t.Error("another key shares the bucket")
	}
	now = now.Add(6 * time.Second)
	if ok, _ := l.Allow("198.51.100.7"); !ok {
		t.Error("no token after 6s")
	}
	// A refused request takes nothing: one token a 6 s is still all that refills.
	if ok, _ := l.Allow("198.51.100.7"); ok {
		t.Error("two tokens after 6s")
	}
}

func TestIdleBucketsAreDropped(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	l := PerMinute(10, 20)
	l.Now = func() time.Time { return now }
	l.Allow("198.51.100.7")
	now = now.Add(idle + time.Second)
	for i := range 1024 {
		l.Allow("203.0.113.9")
		_ = i
	}
	if l.Len() != 1 {
		t.Errorf("keys = %d; the idle one should be gone", l.Len())
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("192.0.2.0/24")}
	for name, c := range map[string]struct{ peer, header, want string }{
		"trusted proxy, header used":     {"127.0.0.1:5555", "198.51.100.7", "198.51.100.7"},
		"trusted range":                  {"192.0.2.10:5555", "2001:db8::1", "2001:db8::1"},
		"untrusted peer, header ignored": {"203.0.113.9:5555", "198.51.100.7", "203.0.113.9"},
		"trusted proxy, garbage header":  {"127.0.0.1:5555", "not-an-ip", "127.0.0.1"},
		"trusted proxy, no header":       {"127.0.0.1:5555", "", "127.0.0.1"},
		"mapped IPv4 peer":               {"[::ffff:127.0.0.1]:5555", "198.51.100.7", "198.51.100.7"},
	} {
		r := httptest.NewRequest("GET", "/auth/login", nil)
		r.RemoteAddr = c.peer
		if c.header != "" {
			r.Header.Set("CF-Connecting-IP", c.header)
		}
		if got := ClientIP(r, trusted); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

func TestHostPrefixesIncludeLoopback(t *testing.T) {
	ps, err := HostPrefixes()
	if err != nil {
		t.Skip("no interfaces:", err)
	}
	for _, p := range ps {
		if p.Contains(netip.MustParseAddr("127.0.0.1")) {
			return
		}
	}
	t.Errorf("host prefixes %v lack 127.0.0.1", ps)
}

func TestViaEdge(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	for name, c := range map[string]struct {
		peer, header string
		want         bool
	}{
		"trusted peer with header":    {"127.0.0.1:1", "198.51.100.7", true},
		"trusted peer without header": {"127.0.0.1:1", "", false},
		"untrusted peer with header":  {"203.0.113.9:1", "198.51.100.7", false},
		"unparseable peer":            {"garbage", "198.51.100.7", false},
	} {
		r := httptest.NewRequest("GET", "/mcp", nil)
		r.RemoteAddr = c.peer
		if c.header != "" {
			r.Header.Set("CF-Connecting-IP", c.header)
		}
		if got := ViaEdge(r, trusted); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func TestRetryAfterIsTheTimeToTheNextToken(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	l := PerMinute(10, 1)
	l.Now = func() time.Time { return now }
	l.Allow("k")
	now = now.Add(2500 * time.Millisecond)
	if ok, wait := l.Allow("k"); ok || wait != 3500*time.Millisecond {
		t.Errorf("ok=%v wait=%v; want refused for 3.5s", ok, wait)
	}
}
