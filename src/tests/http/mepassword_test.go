package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if envelope.Error.Code != want {
		t.Errorf("code = %q, want %q", envelope.Error.Code, want)
	}
}

func TestChangeMyPasswordRequiresCurrent(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	// Wrong current password: same honest answer login gives.
	rec := doRequest(t, h, http.MethodPut, "/v1/me/password", accountID, `{"currentPassword":"errada1234*","newPassword":"nuevaclave9"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), "users.err_invalid_credentials")
}

func TestChangeMyPasswordRejectsWeakPasswords(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	// Current password is right, but the new one is too weak.
	rec := doRequest(t, h, http.MethodPut, "/v1/me/password", accountID, `{"currentPassword":"docente123*","newPassword":"corta1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), "users.err_weak_password")
}

func TestChangeMyPasswordSwapsAndLocksOldOut(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	rec := doRequest(t, h, http.MethodPut, "/v1/me/password", accountID, `{"currentPassword":"docente123*","newPassword":"nuevaclave9"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if _, err := usersSvc.Authenticate(ctx, "carlos.mendoza", "nuevaclave9"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := usersSvc.Authenticate(ctx, "carlos.mendoza", "docente123*"); err == nil {
		t.Fatal("old password still authenticates")
	}
}

func TestChangeMyPasswordWithoutAccountFailsHonest(t *testing.T) {
	h, _, _, _ := meProfileStack(t)

	// Dev identity without a users row: 404, nothing to change.
	rec := doRequest(t, h, http.MethodPut, "/v1/me/password", unknownSubject, `{"currentPassword":"docente123*","newPassword":"nuevaclave9"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertErrorCode(t, rec.Body.Bytes(), "users.err_not_found")
}
