package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/auth"
	"grade/src/core/roles"
	"grade/src/core/teachers"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func accountStack(t *testing.T) (http.Handler, *users.Service, *teachers.Service, string) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	tokens, err := auth.NewService("account-suite-secret", 15*time.Minute, 7*24*time.Hour, nil)
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
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Users:            usersSvc,
		Teachers:         teachersSvc,
		Tokens:           tokens,
		AllowDevIdentity: true,
	}, newI18n(t))
	return h, usersSvc, teachersSvc, admin.ID
}

func TestTeacherAccountResolvesLogin(t *testing.T) {
	h, usersSvc, teachersSvc, adminID := accountStack(t)
	ctx := context.Background()

	tc, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "1122334455",
	})
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	// Without an account the answer is an honest 404, not an error.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/"+tc.ID+"/account", adminID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unlinked code = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "users.err_not_found" {
		t.Fatalf("code = %q, want users.err_not_found", got)
	}

	account, err := usersSvc.Create(ctx, "carlos.mendoza", "carlos@colegio.edu", "docente123*")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := usersSvc.LinkTeacher(ctx, account.ID, tc.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+tc.ID+"/account", adminID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("linked code = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Unknown teacher keeps the teacher 404 (not the account one).
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/00000000-0000-4000-8000-000000000000/account", adminID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown teacher code = %d, want 404", rec.Code)
	}

	// Without manage-users the route is forbidden, never leaked.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+tc.ID+"/account", "11111111-1111-4111-8111-111111111111", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unprivileged code = %d, want 403", rec.Code)
	}
}

func TestCreateUserLinksTeacher(t *testing.T) {
	h, _, teachersSvc, adminID := accountStack(t)
	ctx := context.Background()

	tc, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Ana", Surnames: "Ríos", DocumentID: "9988776655",
	})
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	body, _ := json.Marshal(map[string]string{
		"username": "ana.rios", "email": "ana@colegio.edu",
		"password": "docente123*", "teacherID": tc.ID,
	})
	rec := doRequest(t, h, http.MethodPost, "/v1/users", adminID, string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+tc.ID+"/account", adminID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("linked code = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	unknown, _ := json.Marshal(map[string]string{
		"username": "otro.user", "email": "otro@colegio.edu",
		"password": "docente123*", "teacherID": "00000000-0000-4000-8000-000000000000",
	})
	rec = doRequest(t, h, http.MethodPost, "/v1/users", adminID, string(unknown))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown teacher code = %d, want 404", rec.Code)
	}
}
