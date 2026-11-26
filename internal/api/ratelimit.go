package api

import (
	"sync"
	"time"
)

// limiter allows n events per key in each window. It is in memory, which
// suits a single server process. Memory is bounded: expired keys are swept
// at most once per window, and past maxKeys the table starts over.
type limiter struct {
	mu        sync.Mutex
	n         int
	window    time.Duration
	maxKeys   int
	now       func() time.Time
	counts    map[string]*windowCount
	nextSweep time.Time
}

type windowCount struct {
	n     int
	reset time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, maxKeys: 100_000, now: time.Now, counts: map[string]*windowCount{}}
}

// allow records an event for key and reports whether it is within the limit.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.After(l.nextSweep) {
		for k, c := range l.counts {
			if now.After(c.reset) {
				delete(l.counts, k)
			}
		}
		l.nextSweep = now.Add(l.window)
	}
	c, ok := l.counts[key]
	if !ok || now.After(c.reset) {
		if len(l.counts) >= l.maxKeys {
			l.counts = map[string]*windowCount{}
		}
		c = &windowCount{reset: now.Add(l.window)}
		l.counts[key] = c
	}
	c.n++
	return c.n <= l.n
}
