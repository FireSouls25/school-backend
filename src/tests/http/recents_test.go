package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/classes"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/students"
	"grade/src/core/teachers"
	"grade/src/core/users"
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

	studentsSvc := students.NewService(students.NewMemoryStore())
	classesSvc := classes.NewService(classes.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), 4)
	now := time.Now().UTC()
	group, err := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: "22222222-2222-4222-8222-222222222222", Grade: 9, GroupNo: 1})
	must(err)
	st, err := studentsSvc.Create(ctx, students.Student{
		Names: "Ana", Surnames: "Gómez", ClassID: group.ID,
		DocumentID: "10204009", Birthdate: now.AddDate(-14, 0, 0),
		Caregiver: students.Guardian{Names: "María Gómez", Phone: "3210204009"},
	})
	must(err)
	sid := st.ID
	prof, err := teachersSvc.Create(ctx, teachers.Teacher{Names: "Carlos", Surnames: "Mendoza", DocumentID: "79800111"})
	must(err)
	// The login subject doubles as author id: link it to the teacher
	// profile so the history shows a name instead of a username.
	adminUser, err := usersSvc.Create(ctx, "admin", "", "admin123*")
	must(err)
	_, err = usersSvc.LinkTeacher(ctx, adminUser.ID, prof.ID)
	must(err)
	_, err = warningsSvc.Issue(ctx, warnings.Input{
		StudentID: sid, ClassID: group.ID, TeacherID: adminUser.ID,
		HappenedAt: now.AddDate(0, 0, -1), Gravity: warnings.GravitySevere,
		Title: "Pelea", Description: "Pelea en el descanso",
		Snapshot: warnings.StudentSnapshot{Names: "Anita", Surnames: "Gómez R.", DocumentID: "123", ClassID: "viejo"},
	})
	must(err)
	_, err = warningsSvc.Issue(ctx, warnings.Input{
		StudentID: sid, ClassID: group.ID, TeacherID: adminUser.ID,
		HappenedAt: now.AddDate(0, 0, -100), Gravity: warnings.GravityMild,
		Title: "Tarde", Description: "Llega tarde",
		Snapshot: warnings.StudentSnapshot{Names: "Ana", Surnames: "Gómez", DocumentID: "123", ClassID: group.ID},
	})
	must(err)
	_, err = incidentsSvc.Report(ctx, sid, group.ID, now.AddDate(0, 0, -1),
		incidents.SeverityOrdinary, "Daño en el pupitre del salón con corrector")
	must(err)

	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Warnings:         warningsSvc,
		Incidents:        incidentsSvc,
		Students:         studentsSvc,
		Classes:          classesSvc,
		Teachers:         teachersSvc,
		Users:            usersSvc,
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
	wrow := warns[0]
	// Live records win over the frozen snapshot: the profile says Ana
	// Gómez while the snapshot kept the stale Anita Gómez R.
	if wrow["StudentName"] != "Ana Gómez" {
		t.Errorf("StudentName = %v, want the live profile name", wrow["StudentName"])
	}
	if wrow["ClassLabel"] != "9-1" {
		t.Errorf("ClassLabel = %v, want 9-1", wrow["ClassLabel"])
	}
	// The author login links to a teacher profile, so its name shows.
	if wrow["AuthorName"] != "Carlos Mendoza" {
		t.Errorf("AuthorName = %v, want the linked teacher", wrow["AuthorName"])
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
