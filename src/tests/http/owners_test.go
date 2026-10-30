package httpapi_test

// Owner console: the central administrator lists every account, creates
// admins and secretaries, and is the only role allowed to delete or
// restore accounts, revoke the admin role, or touch other owners.
// Deletion is soft with a retention window; a purge removes the row.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/audit"
	"grade/src/core/auth"
	"grade/src/core/roles"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func ownerStack(t *testing.T) (http.Handler, *users.Service, *roles.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	tokens, err := auth.NewService("owner-suite-secret", time.Hour, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(mustCreateUser(ctx, usersSvc, roleSvc, "owner", "owner@colegio.edu", "owner123*", roles.RoleOwner))
	must(mustCreateUser(ctx, usersSvc, roleSvc, "admin", "admin@colegio.edu", "admin123*", roles.RoleAdmin))
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Tokens:           tokens,
		Audit:            audit.NewService(audit.NewMemoryStore()),
		AllowDevIdentity: true,
	}, newI18n(t)), usersSvc, roleSvc
}

func userID(t *testing.T, usersSvc *users.Service, username string) string {
	t.Helper()
	u, err := usersSvc.ByUsername(context.Background(), username)
	if err != nil {
		t.Fatalf("ByUsername %s: %v", username, err)
	}
	return u.ID
}

func mustCreateUser(ctx context.Context, usersSvc *users.Service, roleSvc *roles.Service, username, email, password string, role roles.Role) error {
	u, err := usersSvc.Create(ctx, username, email, password)
	if err != nil {
		return err
	}
	return roleSvc.Assign(ctx, u.ID, role)
}

func createAPIUser(t *testing.T, h http.Handler, subject, username, role string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "temporal123*", "role": role})
	rec := doRequest(t, h, http.MethodPost, "/v1/users", subject, string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s status = %d, want 201: %s", username, rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("decode created user: %v", err)
	}
	return out.ID
}

func listAPIUsers(t *testing.T, h http.Handler, subject, query string) []map[string]any {
	t.Helper()
	rec := doRequest(t, h, http.MethodGet, "/v1/users"+query, subject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out
}

func TestOwnerSeesAdminsAndSecretaries(t *testing.T) {
	h, usersSvc, _ := ownerStack(t)
	owner := userID(t, usersSvc, "owner")
	admin := userID(t, usersSvc, "admin")

	// The fixture seeds owner + admin; add a secretary through the API.
	createAPIUser(t, h, owner, "secretaria", "secretary")

	asAdmin := listAPIUsers(t, h, admin, "")
	names := map[string]bool{}
	for _, u := range asAdmin {
		names[u["Username"].(string)] = true
	}
	for _, want := range []string{"owner", "admin", "secretaria"} {
		if !names[want] {
			t.Errorf("admin list misses %q: %v", want, names)
		}
	}

	// Secretaries never reach the console roster.
	secID := ""
	for _, u := range asAdmin {
		if u["Username"] == "secretaria" {
			secID, _ = u["ID"].(string)
		}
	}
	if secID == "" {
		t.Fatal("secretary missing from admin list")
	}
	rec := doRequest(t, h, http.MethodGet, "/v1/users", secID, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("secretary list status = %d, want 403", rec.Code)
	}

	// Role filter narrows the roster.
	onlyAdmins := listAPIUsers(t, h, admin, "?role=admin")
	if len(onlyAdmins) != 1 || onlyAdmins[0]["Username"] != "admin" {
		t.Errorf("role=admin list = %v, want just the admin", onlyAdmins)
	}

	// Detail carries roles and no hash.
	rec = doRequest(t, h, http.MethodGet, "/v1/users/"+secID, admin, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if _, leaked := detail["PasswordHash"]; leaked {
		t.Error("detail leaks PasswordHash")
	}
}

func TestAdminCreatesAdminsButCannotDelete(t *testing.T) {
	h, usersSvc, _ := ownerStack(t)
	owner := userID(t, usersSvc, "owner")
	admin := userID(t, usersSvc, "admin")

	target := createAPIUser(t, h, admin, "otro.admin", "admin")

	// Deleting any account is owner-only.
	rec := doRequest(t, h, http.MethodDelete, "/v1/users/"+target, admin, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin delete status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodDelete, "/v1/users/"+target, owner, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("owner delete status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var deleted map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("decode deleted: %v", err)
	}
	if deleted["DeletedAt"] == nil {
		t.Error("delete response misses DeletedAt")
	}
	if deleted["Active"] != false {
		t.Errorf("deleted Active = %v, want false", deleted["Active"])
	}

	// The login dies immediately while the row survives for the owner.
	if _, _, code := loginRaw(t, h, "otro.admin", "temporal123*"); code != http.StatusUnauthorized {
		t.Errorf("deleted login code = %d, want 401", code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/users/"+target, admin, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("admin detail of deleted status = %d, want 404", rec.Code)
	}
	visible := listAPIUsers(t, h, admin, "")
	for _, u := range visible {
		if u["ID"] == target {
			t.Error("deleted account still listed for admin")
		}
	}
	// Owners see the retention bin with the flag.
	bin := listAPIUsers(t, h, owner, "?includeDeleted=true")
	found := false
	for _, u := range bin {
		if u["ID"] == target {
			found = true
			if u["DeletedAt"] == nil {
				t.Error("bin row misses DeletedAt")
			}
		}
	}
	if !found {
		t.Error("deleted account missing from owner bin")
	}
	// ... and admins may not even ask for it.
	rec = doRequest(t, h, http.MethodGet, "/v1/users?includeDeleted=true", admin, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("admin bin status = %d, want 403", rec.Code)
	}

	// Restore brings the login back.
	rec = doRequest(t, h, http.MethodPost, "/v1/users/"+target+"/restore", owner, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	loginToken(t, h, "otro.admin", "temporal123*")
	// Restoring twice is a clear no-op, not a silent one.
	rec = doRequest(t, h, http.MethodPost, "/v1/users/"+target+"/restore", owner, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("second restore status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "users.err_not_deleted" {
		t.Errorf("code = %q, want users.err_not_deleted", code)
	}
}

func TestDeleteProtection(t *testing.T) {
	h, usersSvc, roleSvc := ownerStack(t)
	owner := userID(t, usersSvc, "owner")
	admin := userID(t, usersSvc, "admin")
	ctx := context.Background()

	ownerUser, err := usersSvc.ByUsername(ctx, "owner")
	if err != nil {
		t.Fatalf("ByUsername owner: %v", err)
	}

	// Owners cannot delete their own login.
	rec := doRequest(t, h, http.MethodDelete, "/v1/users/"+ownerUser.ID, owner, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("self delete status = %d, want 403", rec.Code)
	}
	if code := errorCode(t, rec); code != "users.err_self_delete" {
		t.Errorf("code = %q, want users.err_self_delete", code)
	}

	// The last owner can neither be deleted nor stripped of the role.
	// (Deleting the sole owner as themselves reports self-delete first,
	// which is the nearer footgun; the guard below covers the last
	// standing owner through role removal.)
	rec = doRequest(t, h, http.MethodDelete, "/v1/users/"+ownerUser.ID, admin, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin delete owner status = %d, want 403", rec.Code)
	}
	body, _ := json.Marshal(map[string]string{"subjectID": ownerUser.ID, "role": "owner"})
	rec = doRequest(t, h, http.MethodDelete, "/v1/roles", owner, string(body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("remove last owner role status = %d, want 409", rec.Code)
	}

	// With a second owner the guard lifts for the first.
	second := createAPIUser(t, h, owner, "owner.dos", "owner")
	if err := roleSvc.Assign(ctx, second, roles.RoleOwner); err != nil {
		t.Fatalf("grant second owner: %v", err)
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/users/"+ownerUser.ID, second, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete with two owners status = %d, want 200", rec.Code)
	}
}

func TestOwnerOnlyGrants(t *testing.T) {
	h, usersSvc, _ := ownerStack(t)
	owner := userID(t, usersSvc, "owner")
	admin := userID(t, usersSvc, "admin")

	target := createAPIUser(t, h, admin, "concardidato", "teacher")

	grant := func(subject, role string) int {
		body, _ := json.Marshal(map[string]string{"subjectID": target, "role": role})
		rec := doRequest(t, h, http.MethodPost, "/v1/roles", subject, string(body))
		return rec.Code
	}
	// Admins may grant teacher and admin, never owner.
	if code := grant(admin, "teacher"); code != http.StatusCreated {
		t.Errorf("admin grant teacher status = %d, want 201", code)
	}
	if code := grant(admin, "admin"); code != http.StatusCreated {
		t.Errorf("admin grant admin status = %d, want 201", code)
	}
	if code := grant(admin, "owner"); code != http.StatusForbidden {
		t.Errorf("admin grant owner status = %d, want 403", code)
	}
	if code := grant(owner, "owner"); code != http.StatusCreated {
		t.Errorf("owner grant owner status = %d, want 201", code)
	}

	// Granting through creation follows the same rule.
	body, _ := json.Marshal(map[string]string{"username": "jefe", "password": "jefe12345*", "role": "owner"})
	rec := doRequest(t, h, http.MethodPost, "/v1/users", admin, string(body))
	if rec.Code != http.StatusForbidden {
		t.Errorf("admin create owner status = %d, want 403", rec.Code)
	}

	// Revoking admin is owner-only as well.
	revoke := func(subject, role string) int {
		body, _ := json.Marshal(map[string]string{"subjectID": target, "role": role})
		rec := doRequest(t, h, http.MethodDelete, "/v1/roles", subject, string(body))
		return rec.Code
	}
	if code := revoke(admin, "teacher"); code != http.StatusOK {
		t.Errorf("admin revoke teacher status = %d, want 200", code)
	}
	if code := revoke(admin, "admin"); code != http.StatusForbidden {
		t.Errorf("admin revoke admin status = %d, want 403", code)
	}
	if code := revoke(owner, "admin"); code != http.StatusOK {
		t.Errorf("owner revoke admin status = %d, want 200", code)
	}
}

func TestDeactivateAdminNeedsOwner(t *testing.T) {
	h, usersSvc, _ := ownerStack(t)
	owner := userID(t, usersSvc, "owner")
	admin := userID(t, usersSvc, "admin")

	target := createAPIUser(t, h, admin, "director.x", "admin")

	toggle := func(subject string, active bool) int {
		body, _ := json.Marshal(map[string]bool{"active": active})
		rec := doRequest(t, h, http.MethodPut, "/v1/users/"+target+"/active", subject, string(body))
		return rec.Code
	}
	if code := toggle(admin, false); code != http.StatusForbidden {
		t.Errorf("admin deactivate admin status = %d, want 403", code)
	}
	if code := toggle(owner, false); code != http.StatusOK {
		t.Errorf("owner deactivate admin status = %d, want 200", code)
	}
	if code := toggle(owner, true); code != http.StatusOK {
		t.Errorf("owner reactivate admin status = %d, want 200", code)
	}
}
