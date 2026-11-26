package api

import (
	"sync"
	"time"
)

// limiter allows n events per key in each window. It is in memory, which
// suits a single server process.
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	now    func() time.Time
	counts map[string]*windowCount
}

type windowCount struct {
	n     int
	reset time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, now: time.Now, counts: map[string]*windowCount{}}
}

// allow records an event for key and reports whether it is within the limit.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.counts) > 10000 {
		for k, c := range l.counts {
			if now.After(c.reset) {
				delete(l.counts, k)
			}
		}
	}
	c, ok := l.counts[key]
	if !ok || now.After(c.reset) {
		c = &windowCount{reset: now.Add(l.window)}
		l.counts[key] = c
	}
	c.n++
	return c.n <= l.n
}
