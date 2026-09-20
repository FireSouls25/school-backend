package attendance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/attendance"
)

func newService() *attendance.Service {
	return attendance.NewService(attendance.NewMemoryStore())
}

func TestReasonParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want attendance.Reason
		ok   bool
	}{
		{"absence", attendance.ReasonAbsence, true},
		{"  EVASION ", attendance.ReasonEvasion, true},
		{"late", attendance.ReasonLate, true},
		{"  ATRASO ", attendance.ReasonLate, true},
		{"inasistencia", attendance.ReasonAbsence, true},
		{"Evasión", attendance.ReasonEvasion, true},
		{"holiday", "", false},
		{"", "", false},
	} {
		got, err := attendance.Parse(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("Parse(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("Parse(%q) expected error", tc.in)
		}
	}
}

func TestRecordValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	day := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	t.Run("empty student", func(t *testing.T) {
		_, err := svc.Record(ctx, " ", "9-1", day, attendance.ReasonAbsence)
		if !errors.Is(err, attendance.ErrInvalidStudent) {
			t.Errorf("error = %v, want ErrInvalidStudent", err)
		}
	})
	t.Run("missing class", func(t *testing.T) {
		_, err := svc.Record(ctx, "00000000-0000-0000-0000-000000000001", " ", day, attendance.ReasonAbsence)
		if !errors.Is(err, attendance.ErrInvalidClass) {
			t.Errorf("error = %v, want ErrInvalidClass", err)
		}
	})
	t.Run("zero date", func(t *testing.T) {
		_, err := svc.Record(ctx, "00000000-0000-0000-0000-000000000001", "9-1", time.Time{}, attendance.ReasonAbsence)
		if !errors.Is(err, attendance.ErrInvalidDate) {
			t.Errorf("error = %v, want ErrInvalidDate", err)
		}
	})
	t.Run("unknown reason", func(t *testing.T) {
		_, err := svc.Record(ctx, "00000000-0000-0000-0000-000000000001", "9-1", day, attendance.Reason("holiday"))
		if !errors.Is(err, attendance.ErrUnknownReason) {
			t.Errorf("error = %v, want ErrUnknownReason", err)
		}
	})
	t.Run("late arrival", func(t *testing.T) {
		r, err := svc.Record(ctx, "00000000-0000-0000-0000-000000000001", "9-1", day, attendance.ReasonLate)
		if err != nil {
			t.Fatalf("Record late: %v", err)
		}
		if r.Reason != attendance.ReasonLate {
			t.Errorf("Reason = %q, want late", r.Reason)
		}
	})
}

func TestHistoryNewestFirst(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	sid := "11111111-1111-1111-1111-111111111111"

	old := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	if _, err := svc.Record(ctx, sid, "8-2", old, attendance.ReasonEvasion); err != nil {
		t.Fatalf("Record old: %v", err)
	}
	if _, err := svc.Record(ctx, sid, "9-1", recent, attendance.ReasonAbsence); err != nil {
		t.Fatalf("Record recent: %v", err)
	}

	recs, err := svc.ForStudent(ctx, sid)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("len(recs) = %d, want 2", len(recs))
	}
	if !recs[0].Date.After(recs[1].Date) {
		t.Errorf("records not ordered newest first: %v then %v", recs[0].Date, recs[1].Date)
	}
	if recs[0].ClassID != "9-1" || recs[1].ClassID != "8-2" {
		t.Errorf("history must keep class per record: %+v", recs)
	}
}
