// Package throttle caps password guesses per key and window with a
// persistent store, so multi-replica deployments and restarts share one
// budget. Denied attempts do not extend the window: over-limit callers
// are rejected without recording, exactly like the former in-memory
// limiter. Attempts double as an auth audit trail (identifier, ip,
// success, time); no new public endpoint reads them yet.
package throttle

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// Attempt is one throttled try.
type Attempt struct {
	Key     string
	IP      string
	Success bool
	At      time.Time
}

// Store is the persistence port. Implementations must be safe for
// concurrent use.
type Store interface {
	// CountSince returns attempts for key newer than cut.
	CountSince(ctx context.Context, key string, cut time.Time) (int, error)
	// Record persists one attempt.
	Record(ctx context.Context, a Attempt) error
	// DeleteBefore purges attempts at or before cut.
	DeleteBefore(ctx context.Context, cut time.Time) error
}

// MemoryStore is an in-memory Store for tests and development.
type MemoryStore struct {
	mu   sync.Mutex
	rows []Attempt
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Compile-time check.
var _ Store = (*MemoryStore)(nil)

// CountSince implements Store.
func (s *MemoryStore) CountSince(_ context.Context, key string, cut time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.rows {
		if a.Key == key && a.At.After(cut) {
			n++
		}
	}
	return n, nil
}

// Record implements Store.
func (s *MemoryStore) Record(_ context.Context, a Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.At.IsZero() {
		a.At = time.Now()
	}
	s.rows = append(s.rows, a)
	return nil
}

// DeleteBefore implements Store.
func (s *MemoryStore) DeleteBefore(_ context.Context, cut time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.rows[:0]
	for _, a := range s.rows {
		if a.At.After(cut) {
			kept = append(kept, a)
		}
	}
	s.rows = kept
	return nil
}

// Service caps attempts per key and window.
type Service struct {
	store Store
}

// NewService wires a Service onto the store. A nil store gets memory.
func NewService(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

// Allow reports whether key may try once more: counts attempts inside
// window, denies without recording when over max, otherwise records
// (with ip/success for audit) and allows. Storage errors fail closed.
func (s *Service) Allow(ctx context.Context, key, ip string, success bool, max int, window time.Duration, now time.Time) bool {
	cut := now.Add(-window)
	n, err := s.store.CountSince(ctx, key, cut)
	if err != nil {
		return false
	}
	if n >= max {
		return false
	}
	_ = s.store.Record(ctx, Attempt{Key: key, IP: ip, Success: success, At: now})
	_ = s.store.DeleteBefore(ctx, cut.Add(-window))
	return true
}

// AllowSimple is Allow without audit fields (tests, non-auth callers).
func (s *Service) AllowSimple(ctx context.Context, key string, max int, window time.Duration, now time.Time) bool {
	return s.Allow(ctx, key, "", true, max, window, now)
}

// Check reports whether key is under max inside window, without
// recording. Storage errors deny (fail closed).
func (s *Service) Check(ctx context.Context, key string, max int, window time.Duration, now time.Time) bool {
	n, err := s.store.CountSince(ctx, key, now.Add(-window))
	if err != nil {
		return false
	}
	return n < max
}

// Record persists one attempt for audit and future windows.
func (s *Service) Record(ctx context.Context, key, ip string, success bool, now time.Time) {
	_ = s.store.Record(ctx, Attempt{Key: key, IP: ip, Success: success, At: now})
	_ = s.store.DeleteBefore(ctx, now.Add(-2*windowDefault))
}

// windowDefault bounds cleanup when callers pass custom windows.
const windowDefault = time.Hour

// ClientIP resolves the caller IP: RemoteAddr by default; when the peer
// is a trusted proxy, the leftmost X-Forwarded-For entry wins. Unknown
// or unparsable values fall back to RemoteAddr (fail closed to a
// throttleable key, never empty).
func ClientIP(remoteAddr, forwardedFor string, trusted *Nets) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	if host == "" {
		host = remoteAddr
	}
	if trusted == nil || !trusted.Contains(host) {
		return host
	}
	first := strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	if first == "" {
		return host
	}
	if ip := net.ParseIP(first); ip == nil {
		return host
	}
	return first
}

// Nets holds trusted proxy IPs/CIDRs for ClientIP.
type Nets struct {
	ips  []net.IP
	nets []*net.IPNet
}

// ParseTrusted parses comma-separated IPs/CIDRs; empty means trust none.
// Invalid entries are returned as errors (fail fast at boot, never guess).
func ParseTrusted(raw string) (*Nets, error) {
	n := &Nets{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			_, ipnet, err := net.ParseCIDR(part)
			if err != nil {
				return nil, err
			}
			n.nets = append(n.nets, ipnet)
			continue
		}
		ip := net.ParseIP(part)
		if ip == nil {
			return nil, &net.ParseError{Type: "IP address", Text: part}
		}
		n.ips = append(n.ips, ip)
	}
	return n, nil
}

// Contains reports whether host is a trusted proxy.
func (n *Nets) Contains(host string) bool {
	if n == nil {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, t := range n.ips {
		if t.Equal(ip) {
			return true
		}
	}
	for _, ipnet := range n.nets {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
