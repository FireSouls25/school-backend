package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/auth"
	"grade/src/core/roles"
	"grade/src/core/throttle"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func sharedThrottleStack(t *testing.T, svc *throttle.Service) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("throttle-suite-secret", 15*time.Minute, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if _, err := usersSvc.Create(ctx, "admin", "admin@colegio.edu", "admin123*"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	if err := roleSvc.Assign(ctx, admin.ID, roles.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		Throttle:         svc,
		AllowDevIdentity: true,
	}, newI18n(t))
}

func tryLogin(h http.Handler, identifier, password string) int {
	body, _ := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "9.9.9.9:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestThrottleBudgetIsShared(t *testing.T) {
	svc := throttle.NewService(throttle.NewMemoryStore())
	h1 := sharedThrottleStack(t, svc)
	h2 := sharedThrottleStack(t, svc)

	for i := 0; i < 10; i++ {
		if code := tryLogin(h1, "admin", "wrong-pass"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d code = %d, want 401", i, code)
		}
	}
	// The 11th guess is denied even on a fresh Router: same budget.
	if code := tryLogin(h2, "admin", "wrong-pass"); code != http.StatusTooManyRequests {
		t.Fatalf("shared-budget code = %d, want 429", code)
	}
}
