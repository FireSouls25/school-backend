package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

)

// A teacher updates their own contact ficha through the self-service
// endpoint: no manage-users permission involved.
func TestUpdateMyTeacherAppliesContactFields(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/me", accountID,
		`{"Phone":"3105559999","Address":"Calle 10 # 5-20","Email":"nuevo@colegio.edu"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got teacherJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Phone != "3105559999" || got.Email != "nuevo@colegio.edu" {
		t.Errorf("contact fields not applied: %+v", got)
	}
	if got.Names != "Carlos" || got.DocumentID != "1122334455" {
		t.Errorf("identity drifted: %+v", got)
	}
	if !got.Active {
		t.Error("active got reset by a self-service update")
	}
}

// Identity, homeroom and active state travel through the admin endpoint
// only: the self-service body rejects them with 400 instead of ignoring.
func TestUpdateMyTeacherRejectsAdminFields(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	for _, body := range []string{
		`{"Names":"Otro"}`,
		`{"Active":false}`,
		`{"HomeroomClassID":"7-1"}`,
	} {
		rec := doRequest(t, h, http.MethodPut, "/v1/teachers/me", accountID, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

// An admin without a linked teacher profile has no own ficha to update.
func TestUpdateMyTeacherForbiddenWithoutProfile(t *testing.T) {
	h, _, _, _ := meProfileStack(t)

	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/me", adminSubject, `{"Phone":"3105559999"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// The self-service endpoint validates like the admin one: a bad sex
// answers 400 without touching the record.
func TestUpdateMyTeacherRejectsInvalidSex(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/me", accountID, `{"Sex":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code := decodeTeacherError(t, rec.Body.Bytes()); code != "teachers.err_invalid_sex" {
		t.Errorf("code = %q, want teachers.err_invalid_sex", code)
	}

}
