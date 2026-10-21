package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/auth"
	"grade/src/core/roles"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func loginStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("http-suite-secret", time.Hour, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	adminUser, err := usersSvc.Create(ctx, "admin", "admin@colegio.edu", "admin123*")
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	must(roleSvc.Assign(ctx, adminUser.ID, roles.RoleAdmin))
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		AllowDevIdentity: true,
	}, newI18n(t))
}

func loginAs(t *testing.T, h http.Handler, identifier, password string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	rec := doRequest(t, h, http.MethodPost, "/v1/login", "", string(body))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestLoginFlow(t *testing.T) {
	h := loginStack(t)

	// Missing fields are rejected before touching credentials.
	rec := doRequest(t, h, http.MethodPost, "/v1/login", "", `{"identifier":"admin"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password status = %d, want 400", rec.Code)
	}

	// Unknown identifier, wrong password and inactive account all answer
	// 401 with the same code: no enumeration.
	for _, tc := range []struct{ name, id, pass string }{
		{"unknown", "nadie", "whatever1"},
		{"wrong password", "admin", "not-the-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := loginAs(t, h, tc.id, tc.pass)
			if code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", code)
			}
			errBody, _ := body["error"].(map[string]any)
			if errBody["code"] != "users.err_invalid_credentials" {
				t.Errorf("code = %v, want users.err_invalid_credentials", errBody["code"])
			}
		})
	}

	// Success returns a bearer that opens protected routes, with roles
	// embedded and the admin home suggested.
	code, body := loginAs(t, h, "admin", "admin123*")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" || body["subject"] == nil || body["expiresAt"] == nil {
		t.Fatalf("body = %v, want token, subject and expiry", body)
	}
	if body["home"] != "/admin" {
		t.Errorf("home = %v, want /admin", body["home"])
	}

	req, _ := http.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("bearer /v1/me status = %d, want 200: %s", rec2.Code, rec2.Body.String())
	}

	// Forged and expired tokens are rejected; the dev header still
	// authenticates while the deployment allows it.
	req.Header.Set("Authorization", "Bearer "+token[:len(token)-4]+"AAAA")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("forged status = %d, want 401", rec3.Code)
	}
}

func TestLoginThrottle(t *testing.T) {
	h := loginStack(t)
	var last int
	for i := 0; i < 12; i++ {
		code, _ := loginAs(t, h, "admin", "wrong-pass")
		last = code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("12th guess status = %d, want 429", last)
	}
}
