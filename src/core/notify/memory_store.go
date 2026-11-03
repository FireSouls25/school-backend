package notify

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory notify.Store for tests and dev.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]Notification
	byDedup map[string]string
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Notification), byDedup: make(map[string]string)}
}

// Compile-time check that the adapter satisfies the port.
var _ Store = (*MemoryStore)(nil)

// Enqueue implements Store.
func (s *MemoryStore) Enqueue(_ context.Context, n Notification) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n.DedupKey != "" {
		if id, ok := s.byDedup[n.DedupKey]; ok {
			if _, kept := s.entries[id]; kept {
				return false, nil
			}
		}
		s.byDedup[n.DedupKey] = n.ID
	}
	s.entries[n.ID] = n
	return true, nil
}

// Claim implements Store.
func (s *MemoryStore) Claim(_ context.Context, limit, maxAttempts int) ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Notification, 0)
	for _, n := range s.entries {
		if n.Status == StatusPending && n.Attempts < maxAttempts {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkSent implements Store.
func (s *MemoryStore) MarkSent(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.entries[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	n.Status = StatusSent
	n.SentAt = &now
	s.entries[id] = n
	return nil
}

// MarkFailed implements Store.
func (s *MemoryStore) MarkFailed(_ context.Context, id string, maxAttempts int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.entries[id]
	if !ok {
		return ErrNotFound
	}
	n.Attempts++
	if n.Attempts >= maxAttempts {
		n.Status = StatusFailed
	}
	s.entries[id] = n
	return nil
}

// Pending returns the pending rows, oldest first. Tests only.
func (s *MemoryStore) Pending() []Notification {
	rows, _ := s.Claim(context.Background(), 0, 1<<30)
	return rows
}
