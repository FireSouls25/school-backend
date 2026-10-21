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
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func revocationStack(t *testing.T) (http.Handler, *users.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("revocation-suite-secret", time.Hour, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if _, err := usersSvc.Create(ctx, "admin", "admin@colegio.edu", "admin123*"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	admin, err := usersSvc.ByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	if err := roleSvc.Assign(ctx, admin.ID, roles.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		AllowDevIdentity: true,
	}, newI18n(t))
	return h, usersSvc
}

func loginToken(t *testing.T, h http.Handler, identifier, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login body = %s, want token", rec.Body.String())
	}
	return out.Token
}

func bearerGet(t *testing.T, h http.Handler, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStaleBearerDiesOnPasswordRotation(t *testing.T) {
	h, usersSvc := revocationStack(t)
	ctx := context.Background()

	old := loginToken(t, h, "admin", "admin123*")
	if rec := bearerGet(t, h, "/v1/me", old); rec.Code != http.StatusOK {
		t.Fatalf("fresh bearer /v1/me = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	if err := usersSvc.ChangePassword(ctx, admin.ID, "admin123*", "nuevaClave9"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if rec := bearerGet(t, h, "/v1/me", old); rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale bearer /v1/me = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	fresh := loginToken(t, h, "admin", "nuevaClave9")
	if rec := bearerGet(t, h, "/v1/me", fresh); rec.Code != http.StatusOK {
		t.Fatalf("rotated bearer /v1/me = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestStaleBearerDiesOnDisable(t *testing.T) {
	h, usersSvc := revocationStack(t)
	ctx := context.Background()

	token := loginToken(t, h, "admin", "admin123*")
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	if _, err := usersSvc.SetActive(ctx, admin.ID, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if rec := bearerGet(t, h, "/v1/me", token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled bearer /v1/me = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}
