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
