package incidents_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/incidents"
)

func newService() *incidents.Service {
	return incidents.NewService(incidents.NewMemoryStore())
}

func TestSeverityParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want incidents.Severity
	}{
		{"leve", incidents.SeverityMinor},
		{"NORMAL", incidents.SeverityOrdinary},
		{"grave", incidents.SeveritySevere},
		{"minor", incidents.SeverityMinor},
	} {
		got, err := incidents.Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := incidents.Parse("extrema"); err == nil {
		t.Error("Parse(\"extrema\") expected error")
	}
	if sevs := incidents.KnownSeverities(); len(sevs) != 3 {
		t.Errorf("KnownSeverities length = %d, want 3", len(sevs))
	}
}

func TestReportValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	t.Run("empty description", func(t *testing.T) {
		_, err := svc.Report(ctx, "00000000-0000-0000-0000-000000000001", "9-1", day, incidents.SeverityMinor, "  ")
		if !errors.Is(err, incidents.ErrEmptyDescription) {
			t.Errorf("error = %v, want ErrEmptyDescription", err)
		}
	})
	t.Run("unknown severity", func(t *testing.T) {
		_, err := svc.Report(ctx, "00000000-0000-0000-0000-000000000001", "9-1", day, incidents.Severity("huge"), "x")
		if !errors.Is(err, incidents.ErrUnknownSeverity) {
			t.Errorf("error = %v, want ErrUnknownSeverity", err)
		}
	})
	t.Run("bad student id", func(t *testing.T) {
		_, err := svc.ForStudent(ctx, "not-a-uuid")
		if !errors.Is(err, incidents.ErrInvalidStudent) {
			t.Errorf("error = %v, want ErrInvalidStudent", err)
		}
	})
}

func TestFaultHistoryNewestFirst(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	sid := "22222222-2222-2222-2222-222222222222"

	old := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	if _, err := svc.Report(ctx, sid, "7-3", old, incidents.SeverityMinor, "llega tarde"); err != nil {
		t.Fatalf("Report old: %v", err)
	}
	if _, err := svc.Report(ctx, sid, "9-1", recent, incidents.SeveritySevere, "pelea en el aula"); err != nil {
		t.Fatalf("Report recent: %v", err)
	}

	faults, err := svc.ForStudent(ctx, sid)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(faults) != 2 {
		t.Fatalf("len(faults) = %d, want 2", len(faults))
	}
	if !faults[0].Date.After(faults[1].Date) {
		t.Errorf("faults not ordered newest first")
	}
	if faults[0].Severity != incidents.SeveritySevere {
		t.Errorf("faults[0].Severity = %q, want severe", faults[0].Severity)
	}
}
