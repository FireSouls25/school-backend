package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/teachers"
)

// teacherJSON mirrors the Go-cased teacher payload the endpoint returns.
type teacherJSON struct {
	ID         string    `json:"ID"`
	Names      string    `json:"Names"`
	Surnames   string    `json:"Surnames"`
	DocumentID string    `json:"DocumentID"`
	Phone      string    `json:"Phone"`
	Email      string    `json:"Email"`
	Birthdate  time.Time `json:"Birthdate"`
	Sex        string    `json:"Sex"`
	Active     bool      `json:"Active"`
}

// seedTeacher records one profile and returns its id, or fails the test.
func seedTeacher(t *testing.T, teachersSvc *teachers.Service) string {
	t.Helper()
	tc, err := teachersSvc.Create(context.Background(), teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "1122334455",
		Phone: "3001234567", Email: "carlos@colegio.edu",
		Birthdate: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		Sex:       teachers.SexMale,
	})
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	return tc.ID
}

func decodeTeacherError(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return envelope.Error.Code
}

func TestUpdateTeacherAppliesOnlySentFields(t *testing.T) {
	h, _, teachersSvc, _ := meProfileStack(t)
	id := seedTeacher(t, teachersSvc)

	// Only phone and sex travel: everything omitted keeps its stored value.
	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/"+id, adminSubject, `{"Phone":"3105551212","Sex":"female"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got teacherJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Phone != "3105551212" || got.Sex != "female" {
		t.Errorf("sent fields = phone %q sex %q", got.Phone, got.Sex)
	}
	if got.Names != "Carlos" || got.Surnames != "Mendoza" || got.DocumentID != "1122334455" || got.Email != "carlos@colegio.edu" {
		t.Errorf("omitted fields drifted: %+v", got)
	}
	if !got.Active {
		t.Error("active got reset by a partial update")
	}
	if got.Birthdate.Format("2006-01-02") != "1990-05-20" {
		t.Errorf("birthdate = %v, want 1990-05-20", got.Birthdate)
	}
}

func TestUpdateTeacherRejectsInvalidSex(t *testing.T) {
	h, _, teachersSvc, _ := meProfileStack(t)
	id := seedTeacher(t, teachersSvc)

	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/"+id, adminSubject, `{"Sex":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code := decodeTeacherError(t, rec.Body.Bytes()); code != "teachers.err_invalid_sex" {
		t.Errorf("code = %q, want teachers.err_invalid_sex", code)
	}
}

func TestUpdateTeacherDeniedWithoutPermission(t *testing.T) {
	h, _, teachersSvc, _ := meProfileStack(t)
	id := seedTeacher(t, teachersSvc)

	// A plain teacher holds no manage-users permission.
	rec := doRequest(t, h, http.MethodPut, "/v1/teachers/"+id, teacherSubject, `{"Active":false}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
