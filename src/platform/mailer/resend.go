// Package mailer delivers transactional mail through Resend. Only the
// recovery link travels here today; the port lives in core/users so the
// domain never imports transport details.
package mailer

import (
	"context"
	"fmt"
	"strings"

	"github.com/resend/resend-go/v4"
)

// ResendMailer is a users.Mailer over the Resend API. The zero client is
// unusable: build it with NewResendMailer.
type ResendMailer struct {
	client *resend.Client
	from   string
}

// NewResendMailer wires a sender on the given API key and verified From
// address (e.g. no-reply@fractalschool.app). Empty values fail fast so a
// half-configured production never boots silently mail-less.
func NewResendMailer(apiKey, from string) (*ResendMailer, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("mailer: api key and from are required")
	}
	return &ResendMailer{client: resend.NewClient(apiKey), from: from}, nil
}

// SendResetLink implements users.Mailer: one link, no token in subject,
// plain-text fallback included for strict inboxes.
func (m *ResendMailer) SendResetLink(ctx context.Context, to, link string) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("mailer: not configured")
	}
	params := &resend.SendEmailRequest{
		From:    m.from,
		To:      []string{to},
		Subject: "Recupera tu contraseña — Observador Estudiantil",
		Html: `<!doctype html><html lang="es"><body style="font-family:sans-serif;color:#222;max-width:560px;margin:0 auto;padding:24px">` +
			`<h2 style="margin:0 0 12px">Recupera tu contraseña</h2>` +
			`<p>Solicitaste restablecer la contraseña de tu cuenta del <strong>Observador Estudiantil</strong>.</p>` +
			`<p><a href="` + link + `" style="display:inline-block;background:#1b6c80;color:#fff;padding:12px 24px;border-radius:8px;text-decoration:none">Elegir una contraseña nueva</a></p>` +
			`<p style="color:#666;font-size:14px">El enlace vence en una hora y solo sirve una vez. Si no fuiste tú, ignora este mensaje.</p>` +
			`</body></html>`,
		Text: "Solicitaste restablecer tu contraseña del Observador Estudiantil.\n\n" +
			"Abre este enlace (vence en una hora, un solo uso):\n" + link + "\n\n" +
			"Si no fuiste tú, ignora este mensaje.",
	}
	sent, err := m.client.Emails.SendWithContext(ctx, params)
	if err != nil {
		return err
	}
	if sent == nil || sent.Id == "" {
		return fmt.Errorf("mailer: empty resend response")
	}
	return nil
}
