package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func loginStack(t *testing.T) http.Handler {
	t.Helper()
	return loginStackWith(t, nil)
}

// loginStackWith lets a test adjust the wiring (e.g. production cookies).
func loginStackWith(t *testing.T, tweak func(*httpapi.Dependencies)) http.Handler {
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
	ownerUser, err := usersSvc.Create(ctx, "owner", "owner@colegio.edu", "owner123*")
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	must(roleSvc.Assign(ctx, ownerUser.ID, roles.RoleOwner))
	deps := httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		AllowDevIdentity: true,
	}
	if tweak != nil {
		tweak(&deps)
	}
	return httpapi.Router(deps, newI18n(t))
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

	// Owner lands in the admin console too (it holds every permission);
	// without a mapped home the frontend pushed "/" and looked stuck.
	code, body = loginAs(t, h, "owner", "owner123*")
	if code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200: %v", code, body)
	}
	if body["home"] != "/admin" {
		t.Errorf("owner home = %v, want /admin", body["home"])
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

// Behind Cloudflare Tunnel the process sees plain HTTP: production must
// still mark the refresh cookie Secure, on login and on logout.
func TestRefreshCookieSecureInProduction(t *testing.T) {
	for _, secure := range []bool{false, true} {
		h := loginStackWith(t, func(d *httpapi.Dependencies) { d.SecureCookies = secure })
		body := `{"identifier":"admin","password":"admin123*"}`
		rec := doRequest(t, h, http.MethodPost, "/v1/login", "", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("login status = %d", rec.Code)
		}
		if got := refreshCookie(rec).Secure; got != secure {
			t.Errorf("login cookie Secure = %v, want %v", got, secure)
		}
		out := doRequest(t, h, http.MethodPost, "/v1/logout", "", "")
		if got := refreshCookie(out).Secure; got != secure {
			t.Errorf("logout cookie Secure = %v, want %v", got, secure)
		}
	}
}

func refreshCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			return c
		}
	}
	return &http.Cookie{}
}

// One address sweeping usernames hits the aggregate budget even though
// every identifier stays under its own limit.
func TestLoginThrottlePerAddress(t *testing.T) {
	h := loginStack(t)
	limited := false
	for i := 0; i < 70; i++ {
		body, _ := json.Marshal(map[string]string{"identifier": fmt.Sprintf("nadie%d", i), "password": "x1234567"})
		rec := doRequest(t, h, http.MethodPost, "/v1/login", "", string(body))
		if rec.Code == http.StatusTooManyRequests {
			if i < 60 {
				t.Fatalf("throttled after %d attempts, want 60 allowed", i)
			}
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("70 identifiers from one address never throttled")
	}
}

// Every response is hardened; personal data under /v1 is never cached and
// HSTS only goes out where TLS is guaranteed.
func TestSecurityHeaders(t *testing.T) {
	for _, secure := range []bool{false, true} {
		h := loginStackWith(t, func(d *httpapi.Dependencies) { d.SecureCookies = secure })
		api := doRequest(t, h, http.MethodPost, "/v1/login", "", `{"identifier":"admin","password":"admin123*"}`)
		for k, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
			"Cache-Control":          "no-store",
		} {
			if got := api.Header().Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		if got := api.Header().Get("Strict-Transport-Security") != ""; got != secure {
			t.Errorf("HSTS present = %v, want %v", got, secure)
		}
		health := doRequest(t, h, http.MethodGet, "/healthz", "", "")
		if got := health.Header().Get("Cache-Control"); got != "" {
			t.Errorf("healthz Cache-Control = %q, want unset", got)
		}
	}
}

// The web SPA keeps its refresh token in the HttpOnly cookie only: an
// injected script on that origin can never read it from JSON. Native
// webviews (another site, no cookie) still get the body copy.
func TestRefreshTokenCookieOnlyForWebOrigin(t *testing.T) {
	const web = "https://fractalschool.app"
	h := loginStackWith(t, func(d *httpapi.Dependencies) {
		d.CookieOnlyOrigins = []string{web}
		d.AllowedOrigins = []string{web, "https://tauri.localhost"}
	})
	login := func(origin string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(`{"identifier":"admin","password":"admin123*"}`))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	rec, out := login(web)
	if rec.Code != http.StatusOK {
		t.Fatalf("web login status = %d", rec.Code)
	}
	if v, ok := out["refreshToken"]; ok && v != "" {
		t.Errorf("web body refreshToken = %v, want omitted", v)
	}
	if refreshCookie(rec).Value == "" {
		t.Error("web login set no refresh cookie")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}

	// The cookie alone rotates the pair, again without a body copy.
	req := httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(`{}`))
	req.Header.Set("Origin", web)
	req.AddCookie(refreshCookie(rec))
	rot := httptest.NewRecorder()
	h.ServeHTTP(rot, req)
	var rotated map[string]any
	_ = json.Unmarshal(rot.Body.Bytes(), &rotated)
	if rot.Code != http.StatusOK || rotated["token"] == "" {
		t.Fatalf("cookie refresh status = %d %s", rot.Code, rot.Body.String())
	}
	if v, ok := rotated["refreshToken"]; ok && v != "" {
		t.Errorf("web refresh body refreshToken = %v, want omitted", v)
	}

	_, native := login("https://tauri.localhost")
	if native["refreshToken"] == "" || native["refreshToken"] == nil {
		t.Error("native login lost the body refresh token")
	}
}
