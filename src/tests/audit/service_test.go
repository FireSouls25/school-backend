package audit_test

import (
	"context"
	"testing"

	"grade/src/core/audit"
)

const (
	adminSubject   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	teacherSubject = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	studentID      = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func newService() *audit.Service { return audit.NewService(audit.NewMemoryStore()) }

func TestRecordStampsAndAssigns(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	e, err := svc.Record(ctx, adminSubject, audit.ActionRead, audit.ResourceStudent, studentID, 200)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if e.ID == "" {
		t.Error("entry has no id")
	}
	if e.HappenedAt.IsZero() {
		t.Error("entry has no timestamp: the Service must stamp it")
	}
	if e.SubjectID != adminSubject || e.Status != 200 {
		t.Errorf("entry = %+v, want subject and status kept", e)
	}
}

func TestRecordRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		subject string
		action  audit.Action
		res     audit.Resource
		want    string
	}{
		{name: "no subject", subject: "", action: audit.ActionRead, res: audit.ResourceStudent, want: "audit.err_invalid_subject"},
		{name: "malformed subject", subject: "nope", action: audit.ActionRead, res: audit.ResourceStudent, want: "audit.err_invalid_subject"},
		{name: "unknown action", subject: adminSubject, action: audit.Action("drop"), res: audit.ResourceStudent, want: "audit.err_invalid_action"},
		{name: "unknown resource", subject: adminSubject, action: audit.ActionRead, res: audit.Resource("secret"), want: "audit.err_invalid_resource"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService().Record(ctx, tt.subject, tt.action, tt.res, studentID, 200)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if code := errCode(err); code != tt.want {
				t.Errorf("code = %q, want %q", code, tt.want)
			}
		})
	}
}

func TestForStudentReturnsNewestFirst(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Record(ctx, adminSubject, audit.ActionRead, audit.ResourceStudent, studentID, 200); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if _, err := svc.Record(ctx, teacherSubject, audit.ActionCreate, audit.ResourceStudent, "", 201); err != nil {
		t.Fatalf("second record: %v", err)
	}

	entries, err := svc.ForStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (only the one about that student)", len(entries))
	}
	if entries[0].SubjectID != adminSubject {
		t.Errorf("subject = %q, want %q", entries[0].SubjectID, adminSubject)
	}

	if _, err := svc.ForStudent(ctx, "not-a-uuid"); err == nil {
		t.Error("want error for malformed student id")
	} else if code := errCode(err); code != "audit.err_invalid_student" {
		t.Errorf("code = %q, want audit.err_invalid_student", code)
	}
}

func TestRecentRespectsLimitAndCap(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	for i := 0; i < 5; i++ {
		if _, err := svc.Record(ctx, adminSubject, audit.ActionRead, audit.ResourceStudent, studentID, 200); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	entries, err := svc.Recent(ctx, 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("entries = %d, want 2", len(entries))
	}

	entries, err = svc.Recent(ctx, audit.MaxRecent+100)
	if err != nil {
		t.Fatalf("Recent over cap: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("entries = %d, want all 5 (cap is above the total)", len(entries))
	}

	if _, err := svc.Recent(ctx, 0); err == nil {
		t.Error("want error for non-positive limit")
	} else if code := errCode(err); code != "audit.err_invalid_limit" {
		t.Errorf("code = %q, want audit.err_invalid_limit", code)
	}
}

// Denied accesses are part of the trail too: a teacher asking for an
// admin-only report left a trace with its 403.
func TestDeniedAccessIsRecorded(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Record(ctx, teacherSubject, audit.ActionRead, audit.ResourceReport, studentID, 403); err != nil {
		t.Fatalf("Record: %v", err)
	}
	entries, err := svc.ForStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != 403 {
		t.Errorf("entries = %+v, want one entry with status 403", entries)
	}
}

func errCode(err error) string {
	type coded interface{ Code() string }
	if c, ok := err.(coded); ok {
		return c.Code()
	}
	return ""
}
