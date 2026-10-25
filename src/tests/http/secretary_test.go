package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// secretaryStack builds on the shared fixture and mints a secretary login
// through the API itself, so the role grant travels the real path.
func secretaryStack(t *testing.T) (http.Handler, string) {
	t.Helper()
	h := testStack(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"secretaria","email":"secretaria@colegio.edu","password":"secretaria123*","role":"secretary"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create secretary status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode secretary: %v", err)
	}
	if created.ID == "" {
		t.Fatal("secretary id is empty")
	}
	return h, created.ID
}

func TestSecretaryHomeIsAdminConsole(t *testing.T) {
	h, secretary := secretaryStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/me", secretary, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", rec.Code)
	}
	var body struct {
		Roles []string `json:"roles"`
		Home  string   `json:"home"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body.Home != "/admin" {
		t.Errorf("home = %q, want /admin", body.Home)
	}
}

// The secretary blocks and re-enables logins, and the attempt is allowed
// through the block-access permission (not manage-users).
func TestSecretaryBlocksTeacher(t *testing.T) {
	h, secretary := secretaryStack(t)

	rec := doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"bloqueado","email":"bloqueado@colegio.edu","password":"bloqueado123*"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create target status = %d: %s", rec.Code, rec.Body.String())
	}
	var target struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &target); err != nil {
		t.Fatalf("decode target: %v", err)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/users/"+target.ID+"/active", secretary, `{"active":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("block status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var blocked struct {
		Active bool `json:"Active"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &blocked); err != nil {
		t.Fatalf("decode blocked: %v", err)
	}
	if blocked.Active {
		t.Error("Active = true, want false after block")
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/users/"+target.ID+"/active", secretary, `{"active":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unblock status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// Admin accounts are untouchable without manage-users: a secretary
// blocking an admin gets 403, an admin succeeds.
func TestSecretaryCannotTouchAdmins(t *testing.T) {
	h, secretary := secretaryStack(t)

	rec := doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"otro.admin","email":"otro.admin@colegio.edu","password":"otroadmin123*","role":"admin"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create admin status = %d: %s", rec.Code, rec.Body.String())
	}
	var target struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &target); err != nil {
		t.Fatalf("decode target: %v", err)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/users/"+target.ID+"/active", secretary, `{"active":false}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("secretary block admin status = %d, want 403", rec.Code)
	}
	if code := errorCode(t, rec); code != "http.err_forbidden" {
		t.Errorf("code = %q, want http.err_forbidden", code)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/users/"+target.ID+"/active", adminSubject, `{"active":false}`)
	if rec.Code != http.StatusOK {
		t.Errorf("admin block admin status = %d, want 200", rec.Code)
	}
}

// Reads the secretary needs: teachers (to find the lockout switch),
// warnings history, dashboard and per-student statistics.
func TestSecretaryReads(t *testing.T) {
	h, secretary := secretaryStack(t)

	for _, target := range []string{
		"/v1/teachers",
		"/v1/teachers/" + teacherSubject,
		"/v1/warnings/recent",
		"/v1/dashboard/summary?year=2026",
		"/v1/statistics/student/" + studentSubject,
	} {
		rec := doRequest(t, h, http.MethodGet, target, secretary, "")
		if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 200 (404 proves reach)", target, rec.Code)
		}
	}
}

// Writes the secretary must never perform: teacher edits, warnings,
// years, enrollments, users and roles.
func TestSecretaryWritesDenied(t *testing.T) {
	h, secretary := secretaryStack(t)
	fake := "00000000-0000-0000-0000-000000000000"

	cases := []struct{ method, target, body string }{
		{http.MethodPut, "/v1/teachers/" + teacherSubject, `{"Phone":"1"}`},
		{http.MethodPost, "/v1/teachers", `{"Names":"A","Surnames":"B","DocumentID":"1"}`},
		{http.MethodPut, "/v1/warnings/" + fake, `{"Title":"x"}`},
		{http.MethodPost, "/v1/warnings/batch", `{}`},
		{http.MethodPost, "/v1/users", `{"username":"x","email":"x@e.co","password":"x1234567*"}`},
		{http.MethodPost, "/v1/schoolyears/" + fake + "/close", `{}`},
		{http.MethodPost, "/v1/classes", `{"grade":7,"groupNo":9}`},
		{http.MethodPost, "/v1/enrollments", `{}`},
	}
	for _, c := range cases {
		rec := doRequest(t, h, c.method, c.target, secretary, c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403", c.method, c.target, rec.Code)
		}
	}
}
