package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Mailer sends transactional email.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// LogMailer writes emails to the log instead of sending them. Development only:
// it logs password-reset links.
type LogMailer struct {
	Log *slog.Logger
}

func (m LogMailer) Send(ctx context.Context, to, subject, body string) error {
	m.Log.InfoContext(ctx, "email (not sent: log mailer)", "to", to, "subject", subject, "body", body)
	return nil
}

// SMTPMailer sends plain-text email through an SMTP server. In local
// development that is Mailpit, which captures everything in a web inbox.
type SMTPMailer struct {
	Addr     string // host:port
	From     string // e.g. "Pantheon Spins <no-reply@pantheon.local>"
	Username string // optional; enables PLAIN auth (requires TLS unless the host is local)
	Password string
}

func (m SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("smtp: bad From address: %w", err)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("smtp: bad recipient: %w", err)
	}
	msg := buildMessage(from, rcpt, subject, body, time.Now())

	var auth smtp.Auth
	if m.Username != "" {
		host, _, _ := net.SplitHostPort(m.Addr)
		auth = smtp.PlainAuth("", m.Username, m.Password, host)
	}
	// net/smtp has no context support; bound the whole exchange instead.
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(m.Addr, auth, from.Address, []string{rcpt.Address}, msg) }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("smtp: send: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("smtp: send: %w", ctx.Err())
	}
}

// buildMessage renders an RFC 5322 message. Header values come from parsed
// addresses and a Q-encoded subject, so they cannot inject extra headers.
func buildMessage(from, to *mail.Address, subject, body string, now time.Time) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := from.Address[strings.LastIndexByte(from.Address, '@')+1:]

	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + to.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + hex.EncodeToString(id) + "@" + domain + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}
