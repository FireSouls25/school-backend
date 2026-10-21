package audit_test

import (
	"context"
	"errors"
	"testing"

	"grade/src/core/audit"
)

func newService() *audit.Service {
	return audit.NewService(audit.NewMemoryStore())
}

func TestRecordValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	subject := "11111111-1111-4111-8111-111111111111"
	resource := "22222222-2222-4222-8222-222222222222"

	base := audit.Entry{
		SubjectID: subject, Action: audit.ActionCreate,
		Resource: audit.ResourceSchedule, ResourceID: resource, Status: 201,
	}
	if _, err := svc.Record(ctx, base); err != nil {
		t.Fatalf("Record: %v", err)
	}

	for _, tc := range []struct {
		name string
		mut  func(*audit.Entry)
		want error
	}{
		{"bad subject", func(e *audit.Entry) { e.SubjectID = "nope" }, audit.ErrInvalidSubject},
		{"bad action", func(e *audit.Entry) { e.Action = "erase" }, audit.ErrInvalidAction},
		{"bad resource", func(e *audit.Entry) { e.Resource = "desk" }, audit.ErrInvalidResource},
		{"bad resource id", func(e *audit.Entry) { e.ResourceID = "nope" }, audit.ErrInvalidResource},
		{"bad status", func(e *audit.Entry) { e.Status = 99 }, audit.ErrInvalidStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mut(&e)
			if _, err := svc.Record(ctx, e); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}

	// Empty resource id stays valid: failed creates have nothing to point at.
	empty := base
	empty.ResourceID = ""
	if _, err := svc.Record(ctx, empty); err != nil {
		t.Errorf("empty resource id error = %v, want nil", err)
	}
}

func TestListNewestFirstWithFilter(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	subject := "33333333-3333-4333-8333-333333333333"
	student := "44444444-4444-4444-8444-444444444444"

	mk := func(resource audit.Resource, student string) audit.Entry {
		return audit.Entry{
			SubjectID: subject, Action: audit.ActionCreate, Resource: resource,
			ResourceID: "55555555-5555-4555-8555-555555555555",
			StudentID:  student, Status: 201,
		}
	}
	if _, err := svc.Record(ctx, mk(audit.ResourceSchedule, "")); err != nil {
		t.Fatalf("Record schedule: %v", err)
	}
	if _, err := svc.Record(ctx, mk(audit.ResourceStudent, student)); err != nil {
		t.Fatalf("Record student: %v", err)
	}

	all, err := svc.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 || all[0].Resource != audit.ResourceStudent {
		t.Errorf("list = %+v, want student row newest", all)
	}

	capped, err := svc.List(ctx, audit.Filter{Limit: 1})
	if err != nil || len(capped) != 1 {
		t.Fatalf("capped = %+v, %v; want 1", capped, err)
	}

	only, err := svc.List(ctx, audit.Filter{Limit: 10, StudentID: student})
	if err != nil {
		t.Fatalf("filtered List: %v", err)
	}
	if len(only) != 1 || only[0].StudentID != student {
		t.Errorf("filtered = %+v, want the student row", only)
	}

	if _, err := svc.List(ctx, audit.Filter{StudentID: "nope"}); !errors.Is(err, audit.ErrInvalidResource) {
		t.Errorf("bad student filter error = %v, want ErrInvalidResource", err)
	}
}
