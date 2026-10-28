package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/core/enrollments"
	"grade/src/core/schoolyears"
)

// enrollRequest places a student in a class-group.
type enrollRequest struct {
	StudentID    string `json:"studentID"`
	ClassGroupID string `json:"classGroupID"`
}

// promotionRequest decides one student's move at year end. toGroupID is
// empty only for graduation (grade 11); the server stamps author and time.
type promotionRequest struct {
	StudentID   string `json:"studentID"`
	FromGroupID string `json:"fromGroupID"`
	ToGroupID   string `json:"toGroupID"`
	Decision    string `json:"decision"`
}

// handleCreateEnrollment enrolls a student, refusing closed years: the
// group resolves to its year and closed years take no new enrollments.
func handleCreateEnrollment(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body enrollRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		group, err := deps.Classes.ByID(r.Context(), strings.TrimSpace(body.ClassGroupID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if _, err := deps.Students.ByID(r.Context(), strings.TrimSpace(body.StudentID)); err != nil {
			WriteError(w, r, err)
			return
		}
		if deps.Years != nil {
			year, err := deps.Years.ByID(r.Context(), group.SchoolYearID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if year.Closed {
				WriteError(w, r, schoolyears.ErrYearClosed)
				return
			}
		}
		e, err := deps.Enrollments.Enroll(r.Context(), enrollments.Enrollment{
			StudentID:    body.StudentID,
			ClassGroupID: body.ClassGroupID,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}
}

// handleListEnrollments lists enrollments by group (roster source) or by
// student (placement history). Exactly one filter is required.
func handleListEnrollments(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		groupID := strings.TrimSpace(q.Get("groupID"))
		studentID := strings.TrimSpace(q.Get("studentID"))
		if (groupID == "") == (studentID == "") {
			WriteError(w, r, errBadRequest)
			return
		}
		if groupID != "" {
			list, err := deps.Enrollments.EnrollmentsForGroup(r.Context(), groupID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if list == nil {
				list = []enrollments.Enrollment{}
			}
			writeJSON(w, http.StatusOK, list)
			return
		}
		list, err := deps.Enrollments.EnrollmentsForStudent(r.Context(), studentID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []enrollments.Enrollment{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleDeleteEnrollment removes an enrollment, for corrections.
func handleDeleteEnrollment(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Enrollments.Unenroll(r.Context(), strings.TrimSpace(chi.URLParam(r, "id"))); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleRecordPromotion decides one student's move and applies it:
// promote/repeat enroll the destination (already-placed is tolerated,
// so retries never fail), repeat bumps RepeatCount, graduate ends the
// lifecycle. Grade 11 only graduates or repeats; graduation needs no
// destination. Refs validate first; effects then record in order.
func handleRecordPromotion(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body promotionRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		decision := enrollments.Decision(strings.ToLower(strings.TrimSpace(body.Decision)))
		if !decision.IsValid() {
			WriteError(w, r, enrollments.ErrInvalidDecision)
			return
		}
		student, err := deps.Students.ByID(r.Context(), strings.TrimSpace(body.StudentID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		from, err := deps.Classes.ByID(r.Context(), strings.TrimSpace(body.FromGroupID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		toID := strings.TrimSpace(body.ToGroupID)
		switch decision {
		case enrollments.DecisionGraduate:
			if from.Grade != 11 {
				WriteError(w, r, enrollments.ErrInvalidPromotion)
				return
			}
			if toID != "" {
				WriteError(w, r, enrollments.ErrInvalidPromotion)
				return
			}
		default:
			if from.Grade == 11 && decision == enrollments.DecisionPromote {
				WriteError(w, r, enrollments.ErrInvalidPromotion)
				return
			}
			// Sin destino todavia: la decision queda registrada y la
			// distribucion la completa despues (tablero de cierre).
			if toID != "" {
				if _, err := deps.Classes.ByID(r.Context(), toID); err != nil {
					WriteError(w, r, err)
					return
				}
			}
		}
		subject, _ := SubjectFrom(r.Context())
		now := time.Now().UTC()
		if decision != enrollments.DecisionGraduate && toID != "" {
			if _, err := deps.Enrollments.Enroll(r.Context(), enrollments.Enrollment{
				StudentID:    student.ID,
				ClassGroupID: toID,
			}); err != nil && !errors.Is(err, enrollments.ErrDuplicateEnrollment) {
				WriteError(w, r, err)
				return
			}
			if decision == enrollments.DecisionRepeat {
				student.RepeatCount++
				if _, err := deps.Students.Update(r.Context(), student); err != nil {
					WriteError(w, r, err)
					return
				}
			}
		} else {
			if _, err := deps.Students.Graduate(r.Context(), student.ID); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		p, err := deps.Enrollments.RecordPromotion(r.Context(), enrollments.Promotion{
			StudentID:   student.ID,
			FromGroupID: from.ID,
			ToGroupID:   toID,
			Decision:    decision,
			DecidedBy:   subject,
			DecidedAt:   now,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}
}

// handleListPromotions lists promotion records by student (history) or
// by origin group (the year-end review: who is decided, who is pending).
func handleListPromotions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		studentID := strings.TrimSpace(q.Get("studentID"))
		groupID := strings.TrimSpace(q.Get("groupID"))
		if (studentID == "") == (groupID == "") {
			WriteError(w, r, errBadRequest)
			return
		}
		if studentID != "" {
			list, err := deps.Enrollments.PromotionsForStudent(r.Context(), studentID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if list == nil {
				list = []enrollments.Promotion{}
			}
			writeJSON(w, http.StatusOK, list)
			return
		}
		list, err := deps.Enrollments.PromotionsForGroup(r.Context(), groupID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []enrollments.Promotion{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}
