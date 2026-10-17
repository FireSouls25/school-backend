package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"grade/src/core/roles"
	"grade/src/core/students"
	httpapi "grade/src/platform/http"
)

// tinyPNG is a 1x1 transparent PNG: enough for content sniffing.
var tinyPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

func photoStack(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	studentsSvc := students.NewService(students.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	st, err := studentsSvc.Create(ctx, students.Student{
		Names: "Ana", Surnames: "Gómez", ClassID: "9-1",
		DocumentID: "1234567890", Caregiver: students.Guardian{Names: "María"},
	})
	must(err)
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		Students:         studentsSvc,
		AllowDevIdentity: true,
	}, newI18n(t)), st.ID
}

func TestStudentPhotoFlow(t *testing.T) {
	h, studentID := photoStack(t)

	// No photo yet: honest 404, not an empty image.
	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentID+"/photo", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing photo status = %d, want 404", rec.Code)
	}

	// Teachers read photos but cannot upload them.
	payload := `{"data":"` + base64.StdEncoding.EncodeToString(tinyPNG) + `"}`
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", teacherSubject, payload)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher upload status = %d, want 403", rec.Code)
	}

	// Upload, read back with detected content type, clear.
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", adminSubject, payload)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("upload status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentID+"/photo", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	var photo map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &photo); err != nil {
		t.Fatalf("decode photo: %v", err)
	}
	if photo["contentType"] != "image/png" {
		t.Errorf("contentType = %q, want image/png", photo["contentType"])
	}
	if photo["data"] != base64.StdEncoding.EncodeToString(tinyPNG) {
		t.Error("photo bytes did not round-trip")
	}

	// Garbage fails fast; over the domain limit (but under the 4 MiB
	// transport cap) answers 413; over the cap itself answers 400.
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", adminSubject, `{"data":"!!!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad base64 status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", adminSubject,
		`{"data":"`+strings.Repeat("A", 3<<20)+`"}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", adminSubject,
		`{"data":"`+strings.Repeat("A", 4<<20)+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("over-cap status = %d, want 400", rec.Code)
	}

	// Unknown students 404 on both verbs.
	rec = doRequest(t, h, http.MethodPut, "/v1/students/00000000-0000-4000-8000-000000000000/photo", adminSubject, payload)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown upload status = %d, want 404", rec.Code)
	}

	// Empty upload clears back to 404.
	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID+"/photo", adminSubject, `{"data":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d, want 204", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentID+"/photo", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after clear status = %d, want 404", rec.Code)
	}
}
