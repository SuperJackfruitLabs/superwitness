// Package ratelimit holds in-process token buckets, one per key (a client IP or a principal),
// and the rule for which address a request came from.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// idle is how long an unused bucket is kept. Every limit here refills completely well inside it,
// so dropping a bucket forgets nothing.
const idle = 10 * time.Minute

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// Limiter allows perMinute requests a minute per key, with bursts up to burst.
type Limiter struct {
	Now func() time.Time

	limit rate.Limit
	burst int
	mu    sync.Mutex
	keys  map[string]*bucket
	calls int
}

func PerMinute(perMinute, burst int) *Limiter {
	return &Limiter{limit: rate.Limit(float64(perMinute) / 60), burst: burst, keys: map[string]*bucket{}}
}

func (l *Limiter) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Allow takes one token from key's bucket. When there is none it returns false and how long
// until there is.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	b := l.keys[key]
	if b == nil {
		b = &bucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.keys[key] = b
	}
	b.seen = now
	r := b.lim.ReserveN(now, 1)
	if !r.OK() {
		return false, time.Minute
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

// Len is the number of keys held, for tests.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}

func (l *Limiter) sweep(now time.Time) {
	if l.calls++; l.calls%1024 != 0 {
		return
	}
	for k, b := range l.keys {
		if now.Sub(b.seen) > idle {
			delete(l.keys, k)
		}
	}
}
