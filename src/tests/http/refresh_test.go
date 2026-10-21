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

func refreshStack(t *testing.T) (http.Handler, *users.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("refresh-suite-secret", 15*time.Minute, 7*24*time.Hour, nil)
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

func loginPair(t *testing.T, h http.Handler) (access, refresh, cookie string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"identifier": "admin", "password": "admin123*"})
	req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token            string `json:"token"`
		RefreshToken     string `json:"refreshToken"`
		RefreshExpiresAt string `json:"refreshExpiresAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" || out.RefreshToken == "" {
		t.Fatalf("login body = %s, want token+refresh", rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatalf("login set no refresh_token cookie")
	}
	return out.Token, out.RefreshToken, cookie
}

func TestLoginIssuesRefreshCookie(t *testing.T) {
	h, _ := refreshStack(t)
	access, refresh, _ := loginPair(t, h)
	if access == refresh {
		t.Fatalf("access and refresh must differ")
	}
	// Access bearer opens /v1/me; refresh bearer must not.
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("access /v1/me = %d, want 200", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+refresh)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh as access /v1/me = %d, want 401", rec.Code)
	}
}

func TestRefreshRotatesAndReusesFails(t *testing.T) {
	h, _ := refreshStack(t)
	_, refresh, cookie := loginPair(t, h)

	rotate := func(tok, ck string) (string, string, int) {
		body, _ := json.Marshal(map[string]string{"refreshToken": tok})
		req := httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if ck != "" {
			req.AddCookie(&http.Cookie{Name: "refresh_token", Value: ck})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out struct {
			Token        string `json:"token"`
			RefreshToken string `json:"refreshToken"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Token, out.RefreshToken, rec.Code
	}

	access2, refresh2, code := rotate("", cookie)
	if code != http.StatusOK || access2 == "" || refresh2 == "" {
		t.Fatalf("cookie rotate code = %d, want 200 with pair", code)
	}
	// Old refresh (cookie or JSON) is now revoked: reuse fails.
	if _, _, code := rotate(refresh, ""); code != http.StatusUnauthorized {
		t.Fatalf("reused JSON refresh code = %d, want 401", code)
	}
	if _, _, code := rotate("", cookie); code != http.StatusUnauthorized {
		t.Fatalf("reused cookie refresh code = %d, want 401", code)
	}
	// The rotated access works.
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+access2)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotated access /v1/me = %d, want 200", rec.Code)
	}
	_ = refresh2
}

func TestLogoutRevokesRefresh(t *testing.T) {
	h, _ := refreshStack(t)
	_, refresh, cookie := loginPair(t, h)

	req := httptest.NewRequest(http.MethodPost, "/v1/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout code = %d, want 204", rec.Code)
	}
	body, _ := json.Marshal(map[string]string{"refreshToken": refresh})
	req = httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout code = %d, want 401", rec.Code)
	}
}

func TestRefreshDiesOnPasswordRotation(t *testing.T) {
	h, usersSvc := refreshStack(t)
	ctx := context.Background()
	_, refresh, _ := loginPair(t, h)
	admin, _ := usersSvc.ByUsername(ctx, "admin")
	if err := usersSvc.ChangePassword(ctx, admin.ID, "admin123*", "nuevaClave99"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"refreshToken": refresh})
	req := httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after rotation code = %d, want 401", rec.Code)
	}
}
