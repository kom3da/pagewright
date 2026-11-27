package api

import (
	"net/http"
	"net/netip"
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

// privateWarnEvery is how often allowByIP may warn about private addresses.
const privateWarnEvery = time.Hour

// allowByIP counts r against the per-address limiter l. When a private or
// loopback address hits the limit, the server likely sees a proxy or the
// Docker gateway instead of the visitor (e.g. IPv6 through docker-proxy),
// so every visitor shares one budget: say so, at most once an hour.
func (s *Server) allowByIP(l *limiter, r *http.Request) bool {
	key := s.clientKey(r)
	if l.allow(key) {
		return true
	}
	if !privateKey(key) {
		return false
	}
	now := time.Now().UnixNano()
	last := s.privateWarnedAt.Load()
	if last != 0 && time.Duration(now-last) < privateWarnEvery {
		return false
	}
	if s.privateWarnedAt.CompareAndSwap(last, now) {
		s.Logger.Warn("回数制限のかかった要求の多くが、プライベートなアドレスから届いています。閲覧者のIPアドレスが見えておらず、閲覧者全員にまとめて制限がかかっているかもしれません（docs/install/https.md）",
			"client", key)
	}
	return false
}

// privateKey reports whether a clientKey (an address or an IPv6 /64) is a
// private or loopback address.
func privateKey(key string) bool {
	var a netip.Addr
	if p, err := netip.ParsePrefix(key); err == nil {
		a = p.Addr()
	} else if a, err = netip.ParseAddr(key); err != nil {
		return false
	}
	return a.IsPrivate() || a.IsLoopback()
}
