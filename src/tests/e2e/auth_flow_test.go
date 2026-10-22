// Package e2e unifica los flujos auth punta a punta bajo go test: levanta
// la app real (composition root + Router, stores en memoria, seed demo) y
// ejercita login, refresh, password, recovery, throttle y bloqueo en un
// solo recorrido, igual que el despliegue. Es la base del CI.
package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grade/src/app"
	"grade/src/platform/config"
	httpapi "grade/src/platform/http"
	"grade/src/platform/i18n"
)

func boot(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	cfg := config.Default()
	application, err := app.New(ctx, cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	if err := application.Prepare(ctx); err != nil {
		t.Fatalf("Prepare (seed demo): %v", err)
	}
	i18nSvc, err := i18n.NewService()
	if err != nil {
		t.Fatalf("i18n: %v", err)
	}
	return httpapi.Router(httpapi.Dependencies{
		Auth:              application.Services.Roles,
		Roles:             application.Services.Roles,
		RolesSvc:          application.Services.Roles,
		Students:          application.Services.Students,
		Teachers:          application.Services.Teachers,
		Users:             application.Services.Users,
		Warnings:          application.Services.Warnings,
		Sessions:          application.Services.Sessions,
		Statistics:        application.Services.Statistics,
		Dashboard:         application.Services.Dashboard,
		Classes:           application.Services.Classes,
		Years:             application.Services.Years,
		Enrollments:       application.Services.Enrollments,
		Schedules:         application.Services.Schedules,
		Incidents:         application.Services.Incidents,
		Audit:             application.Services.Audit,
		Tokens:            application.Services.Auth,
		Throttle:          application.Services.Throttle,
		Recovery:          application.Services.Recovery,
		RecoveryLogTokens: false,
		AllowDevIdentity:  true,
		Subjects:          application.Services.Subjects,
		TAttendance:       application.Services.TAttendance,
	}, i18nSvc)
}

func do(t *testing.T, h http.Handler, method, target, token, body string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func login(t *testing.T, h http.Handler, identifier, password string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	return do(t, h, http.MethodPost, "/v1/login", "", string(body))
}

// TestAuthFlow recorre el contrato auth contra la app real.
func TestAuthFlow(t *testing.T) {
	h := boot(t)

	code, admin := login(t, h, "admin", "admin123*")
	if code != http.StatusOK || admin["refreshToken"] == nil {
		t.Fatalf("admin login = %d, want 200 con refresh: %v", code, admin)
	}
	if code, me := do(t, h, http.MethodGet, "/v1/me", admin["token"].(string), ""); code != http.StatusOK || me["username"] != "admin" {
		t.Fatalf("admin me = %d, want 200 admin: %v", code, me)
	}

	code, doc := login(t, h, "carlos.mendoza", "docente123*")
	if code != http.StatusOK {
		t.Fatalf("docente login = %d, want 200", code)
	}
	dToken, dRefresh := doc["token"].(string), doc["refreshToken"].(string)
	if code, me := do(t, h, http.MethodGet, "/v1/me", dToken, ""); code != http.StatusOK || me["teacher"] == nil {
		t.Fatalf("docente me = %d, want 200 con teacher: %v", code, me)
	}

	code, pair := do(t, h, http.MethodPost, "/v1/refresh", "", `{"refreshToken":"`+dRefresh+`"}`)
	if code != http.StatusOK || pair["token"] == nil || pair["refreshToken"] == nil {
		t.Fatalf("refresh = %d, want 200 con par: %v", code, pair)
	}
	if code, _ := do(t, h, http.MethodPost, "/v1/refresh", "", `{"refreshToken":"`+dRefresh+`"}`); code != http.StatusUnauthorized {
		t.Fatalf("refresh reusado = %d, want 401", code)
	}

	pwBody, _ := json.Marshal(map[string]string{"currentPassword": "docente123*", "newPassword": "Docente99x"})
	if code, _ := do(t, h, http.MethodPut, "/v1/me/password", dToken, string(pwBody)); code != http.StatusNoContent {
		t.Fatalf("password = %d, want 204", code)
	}
	if code, _ := do(t, h, http.MethodGet, "/v1/me", dToken, ""); code != http.StatusUnauthorized {
		t.Fatalf("access viejo = %d, want 401", code)
	}
	if code, _ := do(t, h, http.MethodPost, "/v1/refresh", "", `{"refreshToken":"`+pair["refreshToken"].(string)+`"}`); code != http.StatusUnauthorized {
		t.Fatalf("refresh tras rotación = %d, want 401", code)
	}
	if code, _ := login(t, h, "carlos.mendoza", "Docente99x"); code != http.StatusOK {
		t.Fatalf("login nueva clave = %d, want 200", code)
	}

	if code, _ := do(t, h, http.MethodPost, "/v1/recover/request", "", `{"identifier":"nadie.existe"}`); code != http.StatusNoContent {
		t.Fatalf("recover desconocido = %d, want 204", code)
	}
	if code, _ := login(t, h, "carlos.mendoza", "Docente99x"); code != http.StatusOK {
		t.Fatalf("re-login = %d, want 200", code)
	}
}

func TestLoginThrottleE2E(t *testing.T) {
	h := boot(t)
	var last int
	for i := 0; i < 12; i++ {
		last, _ = login(t, h, "nadie.throttle", "wrongpass1")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("intento 12 = %d, want 429", last)
	}
}
