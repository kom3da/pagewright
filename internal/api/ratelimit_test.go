package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := newLimiter(2, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	got := []bool{l.allow("a"), l.allow("a"), l.allow("a")}
	if !got[0] || !got[1] || got[2] {
		t.Fatalf("allow a three times = %v, want true, true, false", got)
	}
	if !l.allow("b") {
		t.Error("other keys are independent")
	}
	now = now.Add(2 * time.Minute)
	if !l.allow("a") {
		t.Error("window did not reset")
	}
	if len(l.counts) != 1 {
		t.Errorf("expired keys kept: %d", len(l.counts))
	}
	l.maxKeys = 3
	for _, k := range []string{"x", "y", "z", "w"} {
		l.allow(k)
	}
	if len(l.counts) > 3 {
		t.Errorf("table grew past maxKeys: %d", len(l.counts))
	}
}

func TestClientKey(t *testing.T) {
	s := &Server{Options: Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}}
	tests := []struct {
		remote, xff, want string
	}{
		{"203.0.113.5:1234", "", "203.0.113.5"},
		{"203.0.113.5:1234", "198.51.100.1", "203.0.113.5"}, // untrusted peer: header ignored
		{"10.0.0.2:1234", "198.51.100.1", "198.51.100.1"},
		{"10.0.0.2:1234", "1.2.3.4, 198.51.100.1, 10.0.0.3", "198.51.100.1"}, // spoofed left part ignored
		{"[2001:db8:1:2:3:4:5:6]:1234", "", "2001:db8:1:2::/64"},
	}
	for _, tt := range tests {
		r := &http.Request{RemoteAddr: tt.remote, Header: http.Header{}}
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := s.clientKey(r); got != tt.want {
			t.Errorf("clientKey(%s, %q) = %s, want %s", tt.remote, tt.xff, got, tt.want)
		}
	}
}

// Hitting a per-address limit from a private address (the Docker gateway,
// say) warns once an hour; public addresses never do.
func TestAllowByIPWarnsAboutPrivateAddresses(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{Options: Options{Logger: slog.New(slog.NewTextHandler(&logs, nil))}}
	l := newLimiter(1, time.Minute)
	hit := func(remote string) {
		for range 3 {
			s.allowByIP(l, &http.Request{RemoteAddr: remote, Header: http.Header{}})
		}
	}
	hit("203.0.113.5:1")
	hit("[2001:db8::1]:1")
	if logs.Len() != 0 {
		t.Fatalf("warned for public addresses:\n%s", logs.String())
	}
	hit("172.18.0.1:1")
	hit("[fd70:6167:6577::1]:1")
	if n := strings.Count(logs.String(), "level=WARN"); n != 1 {
		t.Errorf("%d warnings, want 1:\n%s", n, logs.String())
	}
	s.privateWarnedAt.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	hit("[fd70:6167:6577::1]:1")
	if n := strings.Count(logs.String(), "level=WARN"); n != 2 {
		t.Errorf("%d warnings after an hour, want 2:\n%s", n, logs.String())
	}
}
