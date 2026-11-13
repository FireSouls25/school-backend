package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/core/audit"
	"grade/src/core/permits"
	"grade/src/core/sessions"
)

// permitRequest is the body for granting a prior permission. The
// granting office is always the authenticated caller, never a body
// field.
type permitRequest struct {
	StudentID    string `json:"studentID"`
	ClassGroupID string `json:"classGroupID"`
	SubjectID    string `json:"subjectID"`
	Date         string `json:"date"`
	Reason       string `json:"reason"`
}

// permitRow is one permit as the student file and the teacher list
// consume it: ids plus resolved names, so screens never show UUIDs.
type permitRow struct {
	ID            string `json:"ID"`
	StudentID     string `json:"StudentID"`
	StudentName   string `json:"StudentName"`
	ClassGroupID  string `json:"ClassGroupID"`
	ClassLabel    string `json:"ClassLabel"`
	SubjectID     string `json:"SubjectID"`
	SubjectName   string `json:"SubjectName"`
	Date          string `json:"Date"`
	Reason        string `json:"Reason"`
	CreatedBy     string `json:"CreatedBy"`
	CreatedByName string `json:"CreatedByName"`
	CreatedAt     string `json:"CreatedAt"`
}

// permitRowOf resolves one permit for display: student, class-group,
// subject and author names. Missing references fall back to the id,
// never to an invented label.
func permitRowOf(ctx context.Context, deps Dependencies, p permits.Permit) permitRow {
	row := permitRow{
		ID:           p.ID,
		StudentID:    p.StudentID,
		ClassGroupID: p.ClassGroupID,
		SubjectID:    p.SubjectID,
		Date:         p.Date.Format("2006-01-02"),
		Reason:       p.Reason,
		CreatedBy:    p.CreatedBy,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
	}
	if deps.Students != nil {
		if st, err := deps.Students.ByID(ctx, p.StudentID); err == nil {
			row.StudentName = strings.TrimSpace(st.Names + " " + st.Surnames)
		}
	}
	if deps.Classes != nil {
		if g, err := deps.Classes.ByID(ctx, p.ClassGroupID); err == nil {
			row.ClassLabel = g.Label()
		}
	}
	if deps.Subjects != nil && p.SubjectID != "" {
		if s, err := deps.Subjects.SubjectByID(ctx, p.SubjectID); err == nil {
			row.SubjectName = s.Name
		}
	}
	row.CreatedByName = revisionAuthor(ctx, deps, p.CreatedBy)
	return row
}

// handleGrantPermit records a prior permission for one student in one
// class-group on one day. Only admin and secretary hold record-defenses,
// which is the same capability that covers a session in hindsight.
func handleGrantPermit(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body permitRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		group, err := deps.Classes.ByID(r.Context(), strings.TrimSpace(body.ClassGroupID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		student, err := deps.Students.ByID(r.Context(), strings.TrimSpace(body.StudentID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if !studentInGroup(r.Context(), deps, group.ID, student.ID) {
			WriteError(w, r, permits.ErrNotEnrolled)
			return
		}
		subject, _ := SubjectFrom(r.Context())
		p, err := deps.Permits.Grant(r.Context(), permits.Draft{
			StudentID:    student.ID,
			ClassGroupID: group.ID,
			SubjectID:    strings.TrimSpace(body.SubjectID),
			Date:         strings.TrimSpace(body.Date),
			Reason:       strings.TrimSpace(body.Reason),
			CreatedBy:    subject,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionCreate, audit.ResourcePermit, p.ID, p.StudentID, http.StatusCreated)
		// A class already called earlier that day must reflect the
		// permission too: the teacher's existing marks cannot outrank it.
		applyPermitToSessions(r.Context(), deps, p, subject)
		writeJSON(w, http.StatusCreated, permitRowOf(r.Context(), deps, p))
	}
}

// handleListPermits returns permits by student (?studentID=) or by
// class-group and day (?classGroupID=&date=). The teacher list uses the
// second form to lock the students covered before class.
func handleListPermits(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		studentID := strings.TrimSpace(q.Get("studentID"))
		if studentID != "" {
			list, err := deps.Permits.ForStudent(r.Context(), studentID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			out := make([]permitRow, 0, len(list))
			for _, p := range list {
				out = append(out, permitRowOf(r.Context(), deps, p))
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		groupID := strings.TrimSpace(q.Get("classGroupID"))
		if groupID == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		if teacherGroupScope(r, w, deps, groupID) {
			return
		}
		day, err := parseDayParam(q.Get("date"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		list, err := deps.Permits.ForGroupOnDate(r.Context(), groupID, day)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]permitRow, 0, len(list))
		for _, p := range list {
			out = append(out, permitRowOf(r.Context(), deps, p))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleRevokePermit removes one prior permission. The office can undo a
// mistake; the audit trail keeps who revoked it.
func handleRevokePermit(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.Permits.Revoke(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionDelete, audit.ResourcePermit, id, "", http.StatusNoContent)
		w.WriteHeader(http.StatusNoContent)
	}
}

// studentInGroup reports whether the student is enrolled in the group.
// A nil enrollments service (partial test wiring) skips the check.
func studentInGroup(ctx context.Context, deps Dependencies, groupID, studentID string) bool {
	if deps.Enrollments == nil {
		return true
	}
	ens, err := deps.Enrollments.EnrollmentsForGroup(ctx, groupID)
	if err != nil {
		return false
	}
	for _, e := range ens {
		if e.StudentID == studentID {
			return true
		}
	}
	return false
}

// applyPermitToSessions folds a fresh permit into every roll call
// already opened for that group and day. Best effort: a failure here
// never undoes the permission itself.
func applyPermitToSessions(ctx context.Context, deps Dependencies, p permits.Permit, author string) {
	if deps.Sessions == nil {
		return
	}
	list, err := deps.Sessions.SessionsForGroup(ctx, p.ClassGroupID)
	if err != nil {
		return
	}
	for _, sess := range list {
		if !sameSchoolDay(sess.Date.In(schoolZone), p.Date) || !p.AppliesTo(sess.ClassGroupID, sess.SubjectID) {
			continue
		}
		_, _ = deps.Sessions.RecordMark(ctx, sess.ID, p.StudentID,
			sessions.MarkAbsencePermission, author, p.Reason)
	}
}

// sameSchoolDay compares two instants by calendar day.
func sameSchoolDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// omittedRow is one schedule block whose class already ended without a
// roll call: the agenda paints it as omitted instead of leaving the day
// silently unregistered.
type omittedRow struct {
	ScheduleEntryID string `json:"ScheduleEntryID"`
	ClassGroupID    string `json:"ClassGroupID"`
	GroupSlug       string `json:"GroupSlug"`
	ClassLabel      string `json:"ClassLabel"`
	SubjectID       string `json:"SubjectID"`
	SubjectName     string `json:"SubjectName"`
	Weekday         int    `json:"Weekday"`
	Start           string `json:"Start"`
	End             string `json:"End"`
	Omitted         bool   `json:"Omitted"`
}

// handleOmittedRollcalls lists the caller's class blocks of a day that
// already ended without any roll call. Only the caller's own schedule
// is read, so the answer never leaks another teacher's classes.
func handleOmittedRollcalls(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := parseDayParam(r.URL.Query().Get("date"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		day = permits.NormalizeDay(day)
		now := time.Now().In(schoolZone)
		today := permits.NormalizeDay(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
		if day.After(today) {
			writeJSON(w, http.StatusOK, []omittedRow{})
			return
		}
		isToday := sameSchoolDay(day, today)
		minutesNow := now.Hour()*60 + now.Minute()

		subject, _ := SubjectFrom(r.Context())
		teacherID := callerTeacherID(r.Context(), deps, subject)
		entries, err := deps.Schedules.EntriesForTeacherOnDay(r.Context(), teacherID, day.Weekday())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		var called []sessions.Session
		if deps.Sessions != nil {
			called, _ = deps.Sessions.SessionsForTeacher(r.Context(), teacherID)
		}
		out := make([]omittedRow, 0)
		for _, e := range entries {
			// Still in progress or about to start: not omitted yet.
			if isToday && minutesNow < int(e.End) {
				continue
			}
			group, err := deps.Classes.ByID(r.Context(), e.ClassGroupID)
			if err != nil {
				continue
			}
			if !schoolDayFor(r.Context(), deps, group.SchoolYearID, day) {
				continue
			}
			if blockWasCalled(called, e.ClassGroupID, e.SubjectID, day) {
				continue
			}
			name := e.SubjectID
			if deps.Subjects != nil {
				if s, err := deps.Subjects.SubjectByID(r.Context(), e.SubjectID); err == nil {
					name = s.Name
				}
			}
			out = append(out, omittedRow{
				ScheduleEntryID: e.ID,
				ClassGroupID:    e.ClassGroupID,
				GroupSlug:       group.Slug,
				ClassLabel:      group.Label(),
				SubjectID:       e.SubjectID,
				SubjectName:     name,
				Weekday:         e.Weekday,
				Start:           e.Start.String(),
				End:             e.End.String(),
				Omitted:         true,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// blockWasCalled reports whether a session already covers the block:
// same group, same day, and a compatible subject. A homeroom session
// (empty subject) or a permit-style call without subject covers it.
func blockWasCalled(list []sessions.Session, groupID, subjectID string, day time.Time) bool {
	for _, sess := range list {
		if sess.ClassGroupID != groupID || !sameSchoolDay(sess.Date.In(schoolZone), day) {
			continue
		}
		if sess.SubjectID == "" || subjectID == "" || sess.SubjectID == subjectID {
			return true
		}
	}
	return false
}

// schoolDayFor reports whether the day belongs to the open school year
// that owns the group. A closed or missing year has no classes to omit.
func schoolDayFor(ctx context.Context, deps Dependencies, yearID string, day time.Time) bool {
	if deps.Years == nil {
		return true
	}
	sy, err := deps.Years.ByID(ctx, yearID)
	if err != nil {
		return true
	}
	if sy.Closed {
		return false
	}
	return sy.IsSchoolDay(day)
}

// callerTeacherID resolves the teacher profile behind the login: the
// account link first, the subject itself otherwise (dev identities whose
// subject already is a teacher id keep working).
func callerTeacherID(ctx context.Context, deps Dependencies, subject string) string {
	if deps.Users != nil {
		if u, err := deps.Users.ByID(ctx, subject); err == nil && u.TeacherID != "" {
			return u.TeacherID
		}
	}
	return subject
}
