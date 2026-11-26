// Package mail sends plain-text mail.
package mail

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
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
}

// Send implements Sender.
func (s SMTP) Send(_ context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("parse from address: %w", err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("parse to address: %w", err)
	}
	return smtp.SendMail(s.Addr, nil, from.Address, []string{to.Address}, Build(s.From, m, time.Now()))
}

// Build renders the message as RFC 5322 bytes with UTF-8 headers and body.
func Build(from string, m Message, now time.Time) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(m.Body)
	return b.Bytes()
}
