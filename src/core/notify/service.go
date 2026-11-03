package notify

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store is the persistence port for the notification outbox.
// Implementations must be safe for concurrent use.
type Store interface {
	// Enqueue persists n as pending. The same DedupKey twice reports
	// inserted=false and keeps the first row: retries and double taps
	// never double-send.
	Enqueue(ctx context.Context, n Notification) (inserted bool, err error)
	// Claim returns up to limit pending rows with attempts below max,
	// oldest first.
	Claim(ctx context.Context, limit, maxAttempts int) ([]Notification, error)
	// MarkSent stamps a row delivered.
	MarkSent(ctx context.Context, id string) error
	// MarkFailed records an attempt; at maxAttempts the row fails.
	MarkFailed(ctx context.Context, id string, maxAttempts int) error
}

// Service queues WhatsApp template notices. Sending itself happens in
// a background loop (Flush) so roll-call saves never wait on Meta.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Draft is one notice request: the recipient, the template and its
// slots, plus the dedup key collapsing repeats.
type Draft struct {
	To       string
	Kind     Kind
	Template string
	Language string
	Params   []string
	DedupKey string
}

// Notify validates and queues one notice. An undialable phone or a
// blank template fails fast; duplicates are silently kept once.
func (s *Service) Notify(ctx context.Context, d Draft) (bool, error) {
	to, ok := NormalizePhone(d.To)
	if !ok {
		return false, ErrInvalidRecipient
	}
	if strings.TrimSpace(d.Template) == "" {
		return false, ErrInvalidTemplate
	}
	lang := strings.TrimSpace(d.Language)
	if lang == "" {
		lang = "es"
	}
	dedup := strings.TrimSpace(d.DedupKey)
	id := uuid.NewString()
	if dedup == "" {
		// No dedup scope: every row is its own key, never colliding.
		dedup = id
	}
	return s.store.Enqueue(ctx, Notification{
		ID:        id,
		To:        to,
		Kind:      d.Kind,
		Template:  strings.TrimSpace(d.Template),
		Language:  lang,
		Params:    append([]string(nil), d.Params...),
		DedupKey:  dedup,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	})
}

// Flush sends up to limit pending rows through sender. A send error
// records an attempt; rows keep retrying until maxAttempts.
func (s *Service) Flush(ctx context.Context, sender Sender, limit, maxAttempts int) (sent, failed int, err error) {
	rows, err := s.store.Claim(ctx, limit, maxAttempts)
	if err != nil {
		return 0, 0, err
	}
	for _, n := range rows {
		if err := sender.Send(ctx, n); err != nil {
			failed++
			if ferr := s.store.MarkFailed(ctx, n.ID, maxAttempts); ferr != nil {
				return sent, failed, ferr
			}
			continue
		}
		sent++
		if err := s.store.MarkSent(ctx, n.ID); err != nil {
			return sent, failed, err
		}
	}
	return sent, failed, nil
}
