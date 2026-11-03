package notify

import (
	"context"
	"strings"
	"time"
)

// Kind classifies the notice: which template family it uses.
type Kind string

const (
	// KindAbsence is a same-day miss reported to the guardian.
	KindAbsence Kind = "absence"
	// KindLate is a same-day late arrival reported to the guardian.
	KindLate Kind = "late"
	// KindLateRecurring fires once a month when lates pile up.
	KindLateRecurring Kind = "late-recurring"
	// KindEvasion is a same-day skip reported like an absence.
	KindEvasion Kind = "evasion"
	// KindWarning carries a llamado de atención (moderate or severe).
	KindWarning Kind = "warning"
)

// Status tracks an outbox row.
type Status string

const (
	// StatusPending waits for the sender loop.
	StatusPending Status = "pending"
	// StatusSent left the provider without error.
	StatusSent Status = "sent"
	// StatusFailed exhausted its attempts.
	StatusFailed Status = "failed"
)

// Notification is one queued WhatsApp template send. Params fill the
// template slots in order; DedupKey collapses retries and double taps
// (student + day + kind) into a single send.
type Notification struct {
	ID        string
	To        string
	Kind      Kind
	Template  string
	Language  string
	Params    []string
	DedupKey  string
	Status    Status
	Attempts  int
	CreatedAt time.Time
	SentAt    *time.Time
}

// Sender delivers one notification through a provider (Meta Cloud API,
// a BSP or a gateway). Implementations must be safe for concurrent use
// and must never log the recipient or the params.
type Sender interface {
	Send(ctx context.Context, n Notification) error
}

// Template names live in Meta (utility category, Spanish). The params
// order is fixed here; editing the template text never touches code.
const (
	// TemplateAbsence: {{1}} student, {{2}} date.
	TemplateAbsence = "falta_hoy"
	// TemplateLate: {{1}} student, {{2}} date.
	TemplateLate = "atraso_hoy"
	// TemplateEvasion: {{1}} student, {{2}} date.
	TemplateEvasion = "evasion_hoy"
	// TemplateLateRecurring: {{1}} student, {{2}} count, {{3}} month.
	TemplateLateRecurring = "atrasos_recurrentes"
	// TemplateWarning: {{1}} student, {{2}} kind label, {{3}} date.
	TemplateWarning = "llamado_atencion"
)

// NormalizePhone maps a guardian phone to E.164. Colombia numbers are
// 10 digits starting with 3 (mobile): they gain +57. Longer numbers
// already carrying a country code only gain the plus. Anything else is
// rejected: the outbox never stores an undialable number.
func NormalizePhone(raw string) (string, bool) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(raw))
	switch {
	case len(digits) == 10 && digits[0] == '3':
		return "+57" + digits, true
	case len(digits) == 12 && strings.HasPrefix(digits, "57"):
		return "+" + digits, true
	default:
		return "", false
	}
}
