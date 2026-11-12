package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Espejo de los cuerpos que arma el SPA (frontend/src/domains/*/api.ts):
// si uno cambia de forma, estas pruebas fallan antes de llegar a producción.

// updateStudent: GET de la ficha completa, se quitan ID y Status, se
// editan campos y se devuelve el resto tal cual (fecha en RFC 3339).
func TestUpdateStudentWithSPABody(t *testing.T) {
	h, studentID := photoStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentID, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	var profile map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decode: %v", err)
	}
	delete(profile, "ID")
	delete(profile, "Status")
	profile["Phone"] = "3001112233"
	profile["BloodType"] = "o+"
	profile["Birthdate"] = "2011-03-15T00:00:00Z"
	profile["MedicalReport"] = "Asma leve"
	body, _ := json.Marshal(profile)

	// Un docente no edita fichas.
	if rec := doRequest(t, h, http.MethodPut, "/v1/students/"+studentID, teacherSubject, string(body)); rec.Code != http.StatusForbidden {
		t.Fatalf("teacher update status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/students/"+studentID, adminSubject, string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin update status = %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["Phone"] != "3001112233" || got["BloodType"] != "O+" || got["MedicalReport"] != "Asma leve" {
		t.Errorf("update not applied: phone=%v blood=%v medical=%v", got["Phone"], got["BloodType"], got["MedicalReport"])
	}
	if !strings.HasPrefix(got["Birthdate"].(string), "2011-03-15") {
		t.Errorf("Birthdate = %v, want 2011-03-15", got["Birthdate"])
	}
	if got["Names"] != "Ana" || got["DocumentID"] != "1234567890" {
		t.Errorf("untouched fields drifted: %v %v", got["Names"], got["DocumentID"])
	}

	// Un campo que el backend no conoce se rechaza: el SPA no puede
	// mandar basura sin que nadie se entere.
	profile["Inventado"] = "x"
	body, _ = json.Marshal(profile)
	if rec := doRequest(t, h, http.MethodPut, "/v1/students/"+studentID, adminSubject, string(body)); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", rec.Code)
	}
}

// updateMyTeacher (Perfil): el input type=date entrega "AAAA-MM-DD". Las
// apps ya instaladas mandan esa forma; la nueva manda RFC 3339.
func TestUpdateMyTeacherBirthdateFromProfilePage(t *testing.T) {
	for _, date := range []string{"1990-05-01", "1990-05-01T00:00:00Z"} {
		h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
		accountID, _ := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)
		rec := doRequest(t, h, http.MethodPut, "/v1/teachers/me", accountID,
			`{"Phone":"3105559999","Email":"nuevo@colegio.edu","Address":"","Birthplace":"Pasto","Birthdate":"`+date+`","Sex":"female","MedicalConditions":""}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", date, rec.Code, rec.Body.String())
		}
		var got map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if b, _ := got["Birthdate"].(string); !strings.HasPrefix(b, "1990-05-01") {
			t.Errorf("%s: Birthdate = %v, want 1990-05-01", date, got["Birthdate"])
		}
	}
}
