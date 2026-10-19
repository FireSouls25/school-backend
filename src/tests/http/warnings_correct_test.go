package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/roles"
	"grade/src/core/warnings"
	httpapi "grade/src/platform/http"
)

func TestWarningCorrectFlow(t *testing.T) {
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	warningsSvc := warnings.NewService(warnings.NewMemoryStore())
	if err := roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin); err != nil {
		t.Fatalf("setup: %v", err)
	}
	issued, err := warningsSvc.Issue(ctx, warnings.Input{
		StudentID:   "11111111-1111-4111-8111-111111111111",
		ClassID:     "9-1",
		TeacherID:   teacherSubject,
		HappenedAt:  time.Now().UTC(),
		Gravity:     warnings.GravityMild,
		Title:       "Interrupción",
		Description: "Hablaba en clase.",
		Snapshot: warnings.StudentSnapshot{
			Names: "Ana", Surnames: "Gómez", DocumentID: "123", ClassID: "9-1",
		},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Warnings:         warningsSvc,
	}, newI18n(t))

	// Unknown ids 404.
	rec := doRequest(t, h, http.MethodPut, "/v1/warnings/00000000-0000-4000-8000-000000000000", adminSubject, `{"title":"X"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", rec.Code)
	}

	// Correct title and gravity: trail records before and after.
	rec = doRequest(t, h, http.MethodPut, "/v1/warnings/"+issued.ID, adminSubject,
		`{"title":"Interrupción grave","gravity":"grave"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated["Title"] != "Interrupción grave" || updated["Gravity"] != "severe" {
		t.Fatalf("updated = %v, want the corrected fields", updated)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/warnings/"+issued.ID+"/revisions", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revisions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var revs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &revs); err != nil {
		t.Fatalf("decode revisions: %v", err)
	}
	if len(revs) != 1 || revs[0]["OldTitle"] != "Interrupción" || revs[0]["NewGravity"] != "severe" {
		t.Fatalf("revisions = %v, want before and after", revs)
	}

	// Saving without changes writes no new revision.
	rec = doRequest(t, h, http.MethodPut, "/v1/warnings/"+issued.ID, adminSubject,
		`{"title":"Interrupción grave","gravity":"grave"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("noop status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/warnings/"+issued.ID+"/revisions", adminSubject, "")
	var revs2 []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &revs2); err != nil {
		t.Fatalf("decode revisions2: %v", err)
	}
	if len(revs2) != 1 {
		t.Fatalf("revisions2 = %v, want no new revision on no-op", revs2)
	}
}
