package mail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBuildEncodesAddresses(t *testing.T) {
	raw := Build("ウェブサイト管理 <no-reply@example.jp>", Message{To: "山田 太郎 <taro@example.jp>", Subject: "件名", Body: "本文"}, time.Unix(0, 0))
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, name, addr string }{
		{"From", "ウェブサイト管理", "no-reply@example.jp"},
		{"To", "山田 太郎", "taro@example.jp"},
	} {
		v := msg.Header.Get(tc.key)
		for _, r := range v {
			if r > 0x7e {
				t.Errorf("%s header is not ASCII: %q", tc.key, v)
				break
			}
		}
		got, err := mail.ParseAddress(v)
		if err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
		if diff := cmp.Diff(&mail.Address{Name: tc.name, Address: tc.addr}, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", tc.key, diff)
		}
	}
}

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

// plainSMTP is an SMTP server without STARTTLS. It reports whether a
// MAIL command arrived.
func plainSMTP(t *testing.T) (addr string, gotMail <-chan bool) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	ch := make(chan bool, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 test\r\n")
		mailed := false
		defer func() { ch <- mailed }()
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				_, _ = fmt.Fprint(conn, "250-test\r\n250 8BITMIME\r\n")
			case strings.HasPrefix(cmd, "MAIL"):
				mailed = true
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()
	return l.Addr().String(), ch
}

func TestRequireStartTLS(t *testing.T) {
	addr, gotMail := plainSMTP(t)
	s := SMTP{Addr: addr, From: "a@example.com", RequireStartTLS: true}
	err := s.Send(context.Background(), Message{To: "b@example.com", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("Send = %v, want a STARTTLS error", err)
	}
	if <-gotMail {
		t.Error("mail was sent without TLS")
	}
}
