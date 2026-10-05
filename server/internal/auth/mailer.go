package auth

import (
	"context"
	"log/slog"
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
