package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set at build time by GoReleaser and the Dockerfile:
//
//	-ldflags "-X main.version=0.1.0 -X main.commit=<sha> -X main.date=<commit time, RFC 3339>"
//
// date is the commit's time, not the build's, so that builds are reproducible.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// versionText is what `pagewright version` prints. A plain `go build` has
// no ldflags, so the commit and its time fall back to the VCS stamp Go records.
func versionText(info *debug.BuildInfo, ok bool) string {
	c, d, modified := commit, date, false
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if c == "" {
					c = s.Value
				}
			case "vcs.time":
				if d == "" {
					d = s.Value
				}
			case "vcs.modified":
				modified = commit == "" && s.Value == "true"
			}
		}
	}
	if c == "" {
		c = "不明"
	} else if modified {
		c += "（未コミットの変更あり）"
	}
	if d == "" {
		d = "不明"
	}
	return fmt.Sprintf("pagewright %s\nコミット: %s\nコミットの日時: %s\nGo: %s %s/%s\n",
		version, c, d, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func printVersion() {
	info, ok := debug.ReadBuildInfo()
	fmt.Print(versionText(info, ok))
}
