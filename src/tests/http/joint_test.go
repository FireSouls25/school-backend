package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestJointWorkshopFlow(t *testing.T) {
	h := scheduleStack(t)

	// Segundo salón para el taller conjunto.
	rec := doRequest(t, h, http.MethodPost, "/v1/classes", adminSubject,
		`{"year":2026,"grade":9,"groupNo":2}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create group status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var group map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &group); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	groupB, _ := group["ID"].(string)

	slot := func(g string) string {
		return `{"classGroupID":"` + g + `","teacherID":"` + teacherSubject + `","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"09:00","end":"10:00"}`
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject, slot(realGroupID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create anchor status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var anchor map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &anchor); err != nil {
		t.Fatalf("decode anchor: %v", err)
	}
	anchorID, _ := anchor["ID"].(string)
	if j, _ := anchor["JointID"].(string); j != "" {
		t.Fatalf("anchor JointID = %q, want empty", j)
	}

	// Sin jointWith el traslape sigue prohibido.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject, slot(groupB))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("overlap status = %d, want 400", rec.Code)
	}
	var problem map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if problem["error"]["code"] != "schedules.err_teacher_conflict" {
		t.Fatalf("code = %q, want teacher conflict", problem["error"]["code"])
	}

	// El sonda expone los choques para la oferta del taller conjunto.
	rec = doRequest(t, h, http.MethodGet,
		"/v1/schedule/conflicts?teacherID="+teacherSubject+"&weekday=1&start=09:00&end=10:00", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("conflicts status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var found []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &found); err != nil {
		t.Fatalf("decode conflicts: %v", err)
	}
	if len(found) != 1 || found[0]["ID"] != anchorID {
		t.Fatalf("conflicts = %v, want the anchor", found)
	}

	// Con jointWith nace el taller: misma llave en ambos.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject,
		`{"classGroupID":"`+groupB+`","teacherID":"`+teacherSubject+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"09:00","end":"10:00","jointWith":["`+anchorID+`"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("joint status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var joined map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &joined); err != nil {
		t.Fatalf("decode joined: %v", err)
	}
	joint, _ := joined["JointID"].(string)
	if joint == "" {
		t.Fatal("joined JointID empty, want shared key")
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/schedule?teacherID="+teacherSubject, adminSubject, "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	linked := 0
	for _, e := range list {
		if e["JointID"] == joint {
			linked++
		}
	}
	if linked != 2 {
		t.Errorf("linked = %d, want anchor + joined sharing %q", linked, joint)
	}

	// Referencia rota: el bloque ya no existe.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject,
		`{"classGroupID":"`+groupB+`","teacherID":"`+teacherSubject+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":2,"start":"07:00","end":"08:00","jointWith":["00000000-0000-0000-0000-000000000000"]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bogus joint status = %d, want 404", rec.Code)
	}
}
