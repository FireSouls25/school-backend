package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/warnings"
	httpapi "grade/src/platform/http"
)

func recentsStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	warningsSvc := warnings.NewService(warnings.NewMemoryStore())
	incidentsSvc := incidents.NewService(incidents.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))

	sid := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	_, err := warningsSvc.Issue(ctx, warnings.Input{
		StudentID: sid, ClassID: "9-1", TeacherID: teacherSubject,
		HappenedAt: now.AddDate(0, 0, -1), Gravity: warnings.GravitySevere,
		Title: "Pelea", Description: "Pelea en el descanso",
		Snapshot: warnings.StudentSnapshot{Names: "Ana", Surnames: "Gómez", DocumentID: "123", ClassID: "9-1"},
	})
	must(err)
	_, err = warningsSvc.Issue(ctx, warnings.Input{
		StudentID: sid, ClassID: "9-1", TeacherID: teacherSubject,
		HappenedAt: now.AddDate(0, 0, -100), Gravity: warnings.GravityMild,
		Title: "Tarde", Description: "Llega tarde",
		Snapshot: warnings.StudentSnapshot{Names: "Ana", Surnames: "Gómez", DocumentID: "123", ClassID: "9-1"},
	})
	must(err)
	_, err = incidentsSvc.Report(ctx, sid, "9-1", now.AddDate(0, 0, -1),
		incidents.SeverityOrdinary, "Daño en el pupitre del salón con corrector")
	must(err)

	return httpapi.Router(httpapi.Dependencies{
		Auth:      roleSvc,
		Roles:     roleSvc,
		RolesSvc:  roleSvc,
		Warnings:  warningsSvc,
		Incidents: incidentsSvc,
	}, newI18n(t))
}

func TestRecentsFeeds(t *testing.T) {
	h := recentsStack(t)

	// Both feeds are admin-only.
	for _, target := range []string{"/v1/warnings/recent", "/v1/incidents/recent?limit=5"} {
		rec := doRequest(t, h, http.MethodGet, target, teacherSubject, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s as teacher status = %d, want 403", target, rec.Code)
		}
	}

	// Bad limits are rejected.
	rec := doRequest(t, h, http.MethodGet, "/v1/warnings/recent?limit=nope", adminSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400", rec.Code)
	}

	// Warnings: only the in-window one, newest first, full shape.
	rec = doRequest(t, h, http.MethodGet, "/v1/warnings/recent?limit=10", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("warnings status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var warns []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &warns); err != nil {
		t.Fatalf("decode warnings: %v", err)
	}
	if len(warns) != 1 || warns[0]["Gravity"] != "severe" || warns[0]["Title"] != "Pelea" {
		t.Fatalf("warnings = %v, want the single fresh severe", warns)
	}

	// Incidents: fault mapped to the history row (title from description).
	rec = doRequest(t, h, http.MethodGet, "/v1/incidents/recent", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("incidents status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var faults []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &faults); err != nil {
		t.Fatalf("decode incidents: %v", err)
	}
	if len(faults) != 1 {
		t.Fatalf("incidents len = %d, want 1", len(faults))
	}
	row := faults[0]
	if row["Severity"] != "ordinary" {
		t.Errorf("severity = %v, want ordinary", row["Severity"])
	}
	title, _ := row["Title"].(string)
	if title == "" {
		t.Error("title is empty, want description head")
	}
	if _, ok := row["HappenedAt"]; !ok {
		t.Error("HappenedAt missing")
	}
}
