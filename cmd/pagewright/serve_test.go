package main

import "testing"

func TestLogPath(t *testing.T) {
	for in, want := range map[string]string{
		"/admin/":                     "/admin/",
		"/p/secret-token/":            "/p/[redacted]/",
		"/p/secret-token/media/a.jpg": "/p/[redacted]/media/a.jpg",
		"/p/secret-token":             "/p/[redacted]",
		"/posts/p/x":                  "/posts/p/x",
	} {
		if got := logPath(in); got != want {
			t.Errorf("logPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListenURL(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8081": "http://127.0.0.1:8081/",
		":8081":          "http://localhost:8081/",
		"0.0.0.0:8081":   "http://localhost:8081/",
		"[::1]:8081":     "http://[::1]:8081/",
	} {
		if got := listenURL(in); got != want {
			t.Errorf("listenURL(%q) = %q, want %q", in, got, want)
		}
	}
}
