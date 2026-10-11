package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"grade/src/core/audit"
	"grade/src/core/roles"
	"grade/src/core/students"
	"grade/src/core/teachers"
	"grade/src/core/warnings"
	httpapi "grade/src/platform/http"
)

// auditStack mirrors testStack with the audit service wired, so the tests can
// assert what the transport recorded.
func auditStack(t *testing.T) (http.Handler, *audit.Service, string) {
	t.Helper()
	ctx := context.Background()

	roleSvc := roles.NewService(roles.NewMemoryStore())
	auditSvc := audit.NewService(audit.NewMemoryStore())
	studentsSvc := students.NewService(students.NewMemoryStore())

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))

	st, err := studentsSvc.Create(ctx, validStudent(""))
	must(err)

	return httpapi.Router(httpapi.Dependencies{
		Auth:     roleSvc,
		Roles:    roleSvc,
		RolesSvc: roleSvc,
		Students: studentsSvc,
		Teachers: teachers.NewService(teachers.NewMemoryStore()),
		Warnings: warnings.NewService(warnings.NewMemoryStore()),
		Audit:    auditSvc,
	}, newI18n(t)), auditSvc, st.ID
}

func TestAuditRecordsStudentReads(t *testing.T) {
	h, auditSvc, studentID := auditStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentID, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	entries, err := auditSvc.ForStudent(context.Background(), studentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.SubjectID != teacherSubject {
		t.Errorf("subject = %q, want %q", e.SubjectID, teacherSubject)
	}
	if e.Action != audit.ActionRead || e.Resource != audit.ResourceStudent {
		t.Errorf("entry = %+v, want read/student", e)
	}
	if e.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", e.Status)
	}
}

// A denial is part of the trail: someone tried to read a file they may not.
func TestAuditRecordsDeniedAccess(t *testing.T) {
	h, auditSvc, studentID := auditStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentID, unknownSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	entries, err := auditSvc.ForStudent(context.Background(), studentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != http.StatusForbidden {
		t.Errorf("entries = %+v, want one entry with status 403", entries)
	}
	if entries[0].SubjectID != unknownSubject {
		t.Errorf("subject = %q, want %q", entries[0].SubjectID, unknownSubject)
	}
}

// Routes that touch no student data must stay out of the trail.
func TestAuditIgnoresUnrelatedRoutes(t *testing.T) {
	h, auditSvc, _ := auditStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/me", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", rec.Code)
	}
	entries, err := auditSvc.Recent(context.Background(), 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want none for GET /v1/me", entries)
	}
}

func TestAuditListingIsAdminOnly(t *testing.T) {
	h, _, studentID := auditStack(t)

	doRequest(t, h, http.MethodGet, "/v1/students/"+studentID, teacherSubject, "")

	rec := doRequest(t, h, http.MethodGet, "/v1/audit", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin audit status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var entries []audit.Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) == 0 {
		t.Error("want the recorded read in the listing, got none")
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/audit", teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher audit status = %d, want 403", rec.Code)
	}
}
