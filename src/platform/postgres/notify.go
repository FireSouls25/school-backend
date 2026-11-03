package postgres

import (
	"context"
	"encoding/json"
	"time"

	"grade/src/core/notify"
)

// NotifyStore implements notify.Store on top of PostgreSQL.
type NotifyStore struct {
	db *DB
}

// NewNotifyStore returns a notify.Store backed by db.
func NewNotifyStore(db *DB) *NotifyStore { return &NotifyStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ notify.Store = (*NotifyStore)(nil)

// notifyColumns is the full column list used by every outbox SELECT.
const notifyColumns = `id, to_phone, kind, template, language, params, dedup_key, status, attempts, created_at, sent_at`

// scanNotification maps the current row (in notifyColumns order) onto a
// Notification.
func scanNotification(scan func(dest ...any) error) (notify.Notification, error) {
	var n notify.Notification
	var params []byte
	var kind, status string
	var sentAt *time.Time
	err := scan(&n.ID, &n.To, &kind, &n.Template, &n.Language, &params, &n.DedupKey, &status, &n.Attempts, &n.CreatedAt, &sentAt)
	if err != nil {
		return notify.Notification{}, err
	}
	n.Kind = notify.Kind(kind)
	n.Status = notify.Status(status)
	n.SentAt = sentAt
	if len(params) > 0 {
		if err := json.Unmarshal(params, &n.Params); err != nil {
			return notify.Notification{}, err
		}
	}
	return n, nil
}

// Enqueue implements notify.Store.
func (s *NotifyStore) Enqueue(ctx context.Context, n notify.Notification) (bool, error) {
	params, err := json.Marshal(append([]string(nil), n.Params...))
	if err != nil {
		return false, err
	}
	tag, err := s.db.pool.Exec(ctx, `
		INSERT INTO notification_outbox (id, to_phone, kind, template, language, params, dedup_key, status, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', 0, now())
		ON CONFLICT (dedup_key) DO NOTHING`,
		n.ID, n.To, string(n.Kind), n.Template, n.Language, params, n.DedupKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Claim implements notify.Store.
func (s *NotifyStore) Claim(ctx context.Context, limit, maxAttempts int) ([]notify.Notification, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+notifyColumns+`
		FROM notification_outbox
		WHERE status = 'pending' AND attempts < $1
		ORDER BY created_at
		LIMIT $2`, maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]notify.Notification, 0)
	for rows.Next() {
		n, err := scanNotification(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkSent implements notify.Store.
func (s *NotifyStore) MarkSent(ctx context.Context, id string) error {
	tag, err := s.db.pool.Exec(ctx,
		`UPDATE notification_outbox SET status = 'sent', sent_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notify.ErrNotFound
	}
	return nil
}

// MarkFailed implements notify.Store.
func (s *NotifyStore) MarkFailed(ctx context.Context, id string, maxAttempts int) error {
	tag, err := s.db.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET attempts = attempts + 1,
		    status = CASE WHEN attempts + 1 >= $2 THEN 'failed' ELSE status END
		WHERE id = $1`, id, maxAttempts)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notify.ErrNotFound
	}
	return nil
}
