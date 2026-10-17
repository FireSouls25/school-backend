package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"grade/src/core/audit"
	"grade/src/core/roles"
	"grade/src/core/subjects"
	httpapi "grade/src/platform/http"
)

func subjectStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	return httpapi.Router(httpapi.Dependencies{
		Auth:     roleSvc,
		Roles:    roleSvc,
		RolesSvc: roleSvc,
		Subjects: subjects.NewService(subjects.NewMemoryStore()),
		Audit:    audit.NewService(audit.NewMemoryStore()),
	}, newI18n(t))
}

func decodeSubjects(t *testing.T, recBody []byte) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(recBody, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return list
}

func TestSubjectsCatalogFlow(t *testing.T) {
	h := subjectStack(t)

	// Teachers read the catalog but cannot write it.
	rec := doRequest(t, h, http.MethodGet, "/v1/subjects", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher list status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/subjects", teacherSubject,
		`{"name":"Arte","grades":[1],"color":"#22C55E"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher create status = %d, want 403", rec.Code)
	}

	// Unknown fields are rejected: strict JSON like every other write.
	rec = doRequest(t, h, http.MethodPost, "/v1/subjects", adminSubject,
		`{"name":"Arte","grades":[1],"color":"#22C55E","bogus":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", rec.Code)
	}

	// Validation errors carry stable codes.
	for _, tc := range []struct{ name, body, code string }{
		{"bad grades", `{"name":"X","grades":[12],"color":"#22C55E"}`, "subjects.err_invalid_grades"},
		{"bad color", `{"name":"Y","grades":[1],"color":"red"}`, "subjects.err_invalid_color"},
		{"blank name", `{"name":"  ","grades":[1]}`, "subjects.err_invalid_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, http.MethodPost, "/v1/subjects", adminSubject, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			var envelope map[string]map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if envelope["error"]["code"] != tc.code {
				t.Errorf("code = %q, want %q", envelope["error"]["code"], tc.code)
			}
		})
	}

	// Create, read back, update (retire), timeline, guarded delete.
	rec = doRequest(t, h, http.MethodPost, "/v1/subjects", adminSubject,
		`{"name":"Arte","code":"ART","description":"Plástica","grades":[1,2,3],"color":"#22C55E"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	id, _ := created["ID"].(string)
	if id == "" || created["Active"] != true {
		t.Fatalf("created = %v, want id and active", created)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/subjects/"+id, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/subjects/"+id, adminSubject,
		`{"name":"Arte","grades":[1,2,3,4],"color":"#22C55E","active":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("retire status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Assign without a start date is rejected; with one it opens.
	teacher := "11111111-1111-1111-1111-111111111111"
	rec = doRequest(t, h, http.MethodPost, "/v1/assignments", adminSubject,
		`{"teacherID":"`+teacher+`","subjectID":"`+id+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dateless assign status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/assignments", adminSubject,
		`{"teacherID":"`+teacher+`","subjectID":"`+id+`","startedAt":"2026-02-01"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var assignment map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &assignment); err != nil {
		t.Fatalf("decode assignment: %v", err)
	}
	assignmentID, _ := assignment["ID"].(string)

	// The subject timeline shows the open entry.
	rec = doRequest(t, h, http.MethodGet, "/v1/subjects/"+id+"/assignments", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline status = %d, want 200", rec.Code)
	}
	if list := decodeSubjects(t, rec.Body.Bytes()); len(list) != 1 {
		t.Fatalf("timeline len = %d, want 1", len(list))
	}

	// Delete is blocked by history; ending then removing unblocks it.
	rec = doRequest(t, h, http.MethodDelete, "/v1/subjects/"+id, adminSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete with history status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/assignments/"+assignmentID+"/end", adminSubject,
		`{"endedAt":"2026-06-30"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("end status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/assignments/"+assignmentID, adminSubject, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d, want 204", rec.Code)
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/subjects/"+id, adminSubject, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/subjects/"+id, adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", rec.Code)
	}

	// The teacher timeline endpoint feeds the teacher ficha.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+teacher+"/assignments", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher timeline status = %d, want 200", rec.Code)
	}
	if list := decodeSubjects(t, rec.Body.Bytes()); len(list) != 0 {
		t.Fatalf("teacher timeline len = %d, want 0 after removal", len(list))
	}

	// Reassign on a fresh subject so the timeline is non-empty, then read it back.
	rec = doRequest(t, h, http.MethodPost, "/v1/subjects", adminSubject,
		`{"name":"Música","grades":[1],"color":"#22C55E"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var second map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	secondID, _ := second["ID"].(string)
	rec = doRequest(t, h, http.MethodPost, "/v1/assignments", adminSubject,
		`{"teacherID":"`+teacher+`","subjectID":"`+secondID+`","startedAt":"2026-02-01"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reassign status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+teacher+"/assignments", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher timeline status = %d, want 200", rec.Code)
	}
	if list := decodeSubjects(t, rec.Body.Bytes()); len(list) != 1 {
		t.Fatalf("teacher timeline len = %d, want 1", len(list))
	}

	// Every catalog write trailed itself: newest first.
	rec = doRequest(t, h, http.MethodGet, "/v1/audit?limit=20", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want 200", rec.Code)
	}
	var trail []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &trail); err != nil {
		t.Fatalf("decode trail: %v", err)
	}
	if len(trail) != 12 {
		t.Fatalf("trail len = %d, want 12 catalog writes (8 ok + 4 denied)", len(trail))
	}
	first, last := trail[0], trail[11]
	if first["Action"] != "create" || first["Resource"] != "assignment" || first["Status"] != float64(201) {
		t.Errorf("trail[0] = %v, want the reassign create", first)
	}
	if last["Action"] != "create" || last["Resource"] != "subject" || last["Status"] != float64(400) {
		t.Errorf("trail[11] = %v, want the first denied create", last)
	}
}
