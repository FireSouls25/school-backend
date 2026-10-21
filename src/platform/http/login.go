package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"grade/src/core/roles"
)

// loginRequest carries the credentials. Identifier is a username or an
// email; lookup rules match users.Authenticate.
type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// loginResponse is what an authenticated caller stores: the bearer for
// every later call, its expiry, and where the SPA should land.
type loginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt string       `json:"expiresAt"`
	Subject   string       `json:"subject"`
	Roles     []roles.Role `json:"roles"`
	Home      string       `json:"home"`
}

// loginLimit caps password guesses per key per window: brute force gets
// expensive while normal typos never notice.
const (
	loginLimitPerMinute = 10
	loginWindow         = time.Minute
)

// loginLimiter is a fixed-window counter per key. One instance lives in
// the route closure, so every Router (production or test) gets isolated
// counters and nothing global leaks between suites.
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: make(map[string][]time.Time)}
}

// allow reports whether key may try once more right now.
func (l *loginLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// errRateLimited renders 429 when the login throttle bites.
var errRateLimited = codedError{"http.err_rate_limited"}

// loginKey scopes throttling to one identifier from one address. Shared
// NATs share a budget per identifier: the standard tradeoff between
// brute-force cost and lockout blast radius at school scale.
func loginKey(r *http.Request, identifier string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return strings.ToLower(strings.TrimSpace(identifier)) + "|" + host
}

// handleLogin exchanges credentials for a bearer token. Unknown
// identifier, wrong password and inactive account all answer 401 with
// the same code (no enumeration); over-limit callers get 429.
func handleLogin(deps Dependencies, limit *loginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body loginRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Identifier) == "" || body.Password == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		if !limit.allow(loginKey(r, body.Identifier)) {
			WriteError(w, r, errRateLimited)
			return
		}
		u, err := deps.Users.Authenticate(r.Context(), body.Identifier, body.Password)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if deps.Tokens == nil {
			WriteError(w, r, errInternal)
			return
		}
		if rs == nil {
			rs = []roles.Role{}
		}
		roleNames := make([]string, 0, len(rs))
		for _, role := range rs {
			roleNames = append(roleNames, string(role))
		}
		token, expiresAt, err := deps.Tokens.Mint(u.ID, roleNames, u.TokenVersion, time.Now())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, loginResponse{
			Token:     token,
			ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
			Subject:   u.ID,
			Roles:     rs,
			Home:      homeFor(rs),
		})
	}
}
