package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"grade/src/core/roles"
	"grade/src/core/users"
)

// seedAccount records an account without a teacher link and returns its id.
func seedAccount(t *testing.T, usersSvc *users.Service, roleSvc *roles.Service) string {
	t.Helper()
	account, err := usersSvc.Create(context.Background(), "gabriela.admin", "gabriela@colegio.edu", "clave123456")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := roleSvc.Assign(context.Background(), account.ID, roles.RoleAdmin); err != nil {
		t.Fatalf("assign admin: %v", err)
	}
	return account.ID
}

func decodeMePhoto(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var photo map[string]string
	if err := json.Unmarshal(body, &photo); err != nil {
		t.Fatalf("decode photo: %v", err)
	}
	return photo
}

func bigPhotoPayload() string {
	// ~3 MiB decoded: past the 2 MiB cap of every photo store.
	return `{"data":"` + base64.StdEncoding.EncodeToString(make([]byte, 3<<20)) + `"}`
}

func TestMyPhotoFallsThroughToAccount(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, teacherID := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	// No photo anywhere: honest 404, no invented image.
	rec := doRequest(t, h, http.MethodGet, "/v1/me/photo", accountID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing photo status = %d, want 404", rec.Code)
	}

	// With a linked teacher the picture lands on the staff profile.
	payload := `{"data":"` + base64.StdEncoding.EncodeToString(tinyPNG) + `"}`
	rec = doRequest(t, h, http.MethodPut, "/v1/me/photo", accountID, payload)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("put status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	stored, err := teachersSvc.Photo(context.Background(), teacherID)
	if err != nil || len(stored.Data) == 0 {
		t.Fatalf("teacher photo = %v, %v", len(stored.Data), err)
	}
	accountPhoto, err := usersSvc.Photo(context.Background(), accountID)
	if err != nil {
		t.Fatalf("account photo error: %v", err)
	}
	if len(accountPhoto.Data) != 0 {
		t.Errorf("account photo should stay empty for linked docentes, got %d bytes", len(accountPhoto.Data))
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/me/photo", accountID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	if got := decodeMePhoto(t, rec.Body.Bytes()); got["contentType"] != "image/png" {
		t.Errorf("contentType = %q, want image/png", got["contentType"])
	}

	// Clearing goes back to the honest 404.
	rec = doRequest(t, h, http.MethodPut, "/v1/me/photo", accountID, `{"data":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d, want 204", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/me/photo", accountID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after clear status = %d, want 404", rec.Code)
	}
}

func TestMyPhotoStoredInAccountWithoutTeacher(t *testing.T) {
	h, usersSvc, _, roleSvc := meProfileStack(t)
	accountID := seedAccount(t, usersSvc, roleSvc)

	payload := `{"data":"` + base64.StdEncoding.EncodeToString(tinyPNG) + `"}`
	rec := doRequest(t, h, http.MethodPut, "/v1/me/photo", accountID, payload)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("put status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	stored, err := usersSvc.Photo(context.Background(), accountID)
	if err != nil || len(stored.Data) == 0 {
		t.Fatalf("account photo = %v, %v", len(stored.Data), err)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/me/photo", accountID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	if got := decodeMePhoto(t, rec.Body.Bytes()); got["contentType"] != "image/png" {
		t.Errorf("contentType = %q, want image/png", got["contentType"])
	}

	// Too big: honest 400 from the policy, nothing stored.
	rec = doRequest(t, h, http.MethodPut, "/v1/me/photo", accountID, bigPhotoPayload())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("big photo status = %d, want 400", rec.Code)
	}
}

func TestMyPhotoDeniedWithoutAccount(t *testing.T) {
	h, _, _, _ := meProfileStack(t)

	// Dev identity: no account row anywhere, PUT cannot find a home.
	rec := doRequest(t, h, http.MethodPut, "/v1/me/photo", unknownSubject, `{"data":""}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unkown put status = %d, want 404", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/me/photo", unknownSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown get status = %d, want 404", rec.Code)
	}
}
