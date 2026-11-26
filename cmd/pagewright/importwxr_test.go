package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestImportWXRArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no file", []string{"--site", "0198f3c2-0000-7000-8000-000000000000", "--author-email", "a@example.com"}, "ファイルを1つ"},
		{"bad site", []string{"--site", "abc", "--author-email", "a@example.com", "x.xml"}, "--site"},
		{"no author", []string{"--site", "0198f3c2-0000-7000-8000-000000000000", "x.xml"}, "--author-email"},
		{"missing file", []string{"--site", "0198f3c2-0000-7000-8000-000000000000", "--author-email", "a@example.com", "testdata-does-not-exist.xml"}, "ファイルを開けません"},
		{"unknown flag", []string{"--nope"}, "-nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := importWXR(context.Background(), tt.args, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
	// runImportWXR is the entry point main wires up.
	if err := runImportWXR(context.Background(), []string{"--site", "abc", "x.xml"}); err == nil {
		t.Error("runImportWXR with a bad site: want error")
	}
}
