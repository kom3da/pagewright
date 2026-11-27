package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionText(t *testing.T) {
	stamped := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc123"},
		{Key: "vcs.time", Value: "2026-11-01T00:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}}
	tests := []struct {
		name                  string
		version, commit, date string
		info                  *debug.BuildInfo
		want                  []string
	}{
		{
			name:    "ldflags win over the VCS stamp",
			version: "0.1.0", commit: "def456", date: "2026-11-27T00:00:00Z",
			info: stamped,
			want: []string{"pagewright 0.1.0\n", "コミット: def456\n", "コミットの日時: 2026-11-27T00:00:00Z\n"},
		},
		{
			name:    "go build falls back to the VCS stamp",
			version: "dev",
			info:    stamped,
			want:    []string{"pagewright dev\n", "コミット: abc123（未コミットの変更あり）\n", "コミットの日時: 2026-11-01T00:00:00Z\n"},
		},
		{
			name:    "nothing known",
			version: "dev",
			want:    []string{"コミット: 不明\n", "コミットの日時: 不明\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { version, commit, date = "dev", "", "" })
			version, commit, date = tt.version, tt.commit, tt.date
			got := versionText(tt.info, tt.info != nil)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("versionText() = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}
