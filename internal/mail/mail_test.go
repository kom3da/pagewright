package mail

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestBuildKeepsLinesShort(t *testing.T) {
	body := strings.Repeat("長い日本語の行", 1000)
	raw := Build("Pagewright <no-reply@example.jp>", Message{To: "a@example.jp", Subject: "件名", Body: body}, time.Unix(0, 0))
	for _, line := range bytes.Split(raw, []byte("\r\n")) {
		if len(line) > 998 {
			t.Fatalf("line of %d bytes", len(line))
		}
	}
	_, enc, _ := strings.Cut(string(raw), "\r\n\r\n")
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(enc, "\r\n", ""))
	if err != nil || string(dec) != body {
		t.Errorf("body does not round-trip: %v", err)
	}
}
