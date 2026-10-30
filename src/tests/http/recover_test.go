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

func recoverStack(t *testing.T) (http.Handler, *users.Service, *users.RecoveryService) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("recover-suite-secret", 15*time.Minute, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	recSvc := users.NewRecoveryService(usersSvc, users.NewMemoryRecoveryStore(), time.Hour)
	if _, err := usersSvc.Create(ctx, "admin", "admin@colegio.edu", "admin123*"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	if err := roleSvc.Assign(ctx, admin.ID, roles.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		Recovery:         recSvc,
		AllowDevIdentity: true,
	}, newI18n(t))
	return h, usersSvc, recSvc
}

func TestRecoverRequestNeverEnumerates(t *testing.T) {
	h, _, _ := recoverStack(t)
	for _, id := range []string{"nadie", "admin", "admin@colegio.edu"} {
		body, _ := json.Marshal(map[string]string{"identifier": id})
		req := httptest.NewRequest(http.MethodPost, "/v1/recover/request", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %q code = %d, want 204", id, rec.Code)
		}
	}
}

func TestRecoverResetRotatesAndKillsSessions(t *testing.T) {
	h, usersSvc, recSvc := recoverStack(t)
	ctx := context.Background()

	oldBearer := loginToken(t, h, "admin", "admin123*")
	oldRefresh := ""
	{
		body, _ := json.Marshal(map[string]string{"identifier": "admin", "password": "admin123*"})
		req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		oldRefresh = out.RefreshToken
	}

	token, err := recSvc.Request(ctx, "admin")
	if err != nil || token == "" {
		t.Fatalf("service request = %q, %v", token, err)
	}
	body, _ := json.Marshal(map[string]string{"token": token, "newPassword": "nuevaClave99"})
	req := httptest.NewRequest(http.MethodPost, "/v1/recover/reset", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reset code = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	// Old bearer dies (version bump), old refresh is revoked.
	if rec := bearerGet(t, h, "/v1/me", oldBearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old bearer /v1/me = %d, want 401", rec.Code)
	}
	{
		b, _ := json.Marshal(map[string]string{"refreshToken": oldRefresh})
		req := httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("old refresh code = %d, want 401", rec.Code)
		}
	}
	// Reuse of the same reset token fails.
	req = httptest.NewRequest(http.MethodPost, "/v1/recover/reset", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse code = %d, want 401", rec.Code)
	}
	// New password logs in.
	_ = usersSvc
	loginToken(t, h, "admin", "nuevaClave99")
}

func TestRecoverResetRejectsWeak(t *testing.T) {
	h, _, recSvc := recoverStack(t)
	token, err := recSvc.Request(context.Background(), "admin")
	if err != nil || token == "" {
		t.Fatalf("request = %q, %v", token, err)
	}
	body, _ := json.Marshal(map[string]string{"token": token, "newPassword": "corta1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/recover/reset", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak code = %d, want 400", rec.Code)
	}
}

func TestSetUserActiveLocksAndRestores(t *testing.T) {
	h, usersSvc, _ := recoverStack(t)
	ctx := context.Background()
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	// The lockout flow runs against a plain account: flipping an admin
	// now requires the owner role (covered in owners_test.go).
	plain, err := usersSvc.Create(ctx, "operador", "operador@colegio.edu", "operador123*")
	if err != nil {
		t.Fatalf("seed plain user: %v", err)
	}
	bearer := loginToken(t, h, "operador", "operador123*")

	toggle := func(active bool) int {
		body, _ := json.Marshal(map[string]bool{"active": active})
		req := httptest.NewRequest(http.MethodPut, "/v1/users/"+plain.ID+"/active", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(httpapi.SubjectHeader, admin.ID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := toggle(false); code != http.StatusOK {
		t.Fatalf("disable code = %d, want 200", code)
	}
	if rec := bearerGet(t, h, "/v1/me", bearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled bearer = %d, want 401", rec.Code)
	}
	if _, _, code := loginRaw(t, h, "operador", "operador123*"); code != http.StatusUnauthorized {
		t.Fatalf("disabled login code = %d, want 401", code)
	}
	if code := toggle(true); code != http.StatusOK {
		t.Fatalf("enable code = %d, want 200", code)
	}
	loginToken(t, h, "operador", "operador123*")
}

func loginRaw(t *testing.T, h http.Handler, identifier, password string) (string, string, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Token, "", rec.Code
}
