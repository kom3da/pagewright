// Package mail sends plain-text mail.
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Message is one plain-text mail.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender sends mail.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTP sends through an SMTP server without authentication (e.g. Mailpit
// in development, or a local relay).
type SMTP struct {
	Addr string // host:port
	From string // e.g. "Pagewright <no-reply@example.com>"
	// Username and Password enable SMTP AUTH (PLAIN). They are only sent
	// over TLS: STARTTLS, or ImplicitTLS.
	Username, Password string
	// ImplicitTLS starts TLS on connect (port 465) instead of STARTTLS.
	ImplicitTLS bool
}

// Send implements Sender. It gives up after 30 seconds or when ctx ends.
func (s SMTP) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("parse from address: %w", err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("parse to address: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	host, _, _ := net.SplitHostPort(s.Addr)
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if s.ImplicitTLS {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", s.Addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
	}
	if err != nil {
		return fmt.Errorf("connect to smtp: %w", err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if !s.ImplicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}
	if s.Username != "" {
		// smtp.PlainAuth refuses to send the password without TLS (except
		// to localhost).
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Build(s.From, m, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Build renders the message as RFC 5322 bytes: UTF-8 headers and a base64
// body, so long Japanese lines stay within SMTP's line limit.
func Build(from string, m Message, now time.Time) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(m.Body, "\n", "\r\n")))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes()
}
