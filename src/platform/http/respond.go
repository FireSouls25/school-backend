package httpapi

import (
	"encoding/json"
	"net/http"

	"grade/src/platform/i18n"
)

// ErrorResponse is the user-facing error envelope sent to clients.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries a stable machine-readable code and a localized message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// statusByCode maps coded domain errors to HTTP status codes.
// Codes not listed fall back to 500.
var statusByCode = map[string]int{
	"roles.err_unknown_role":    http.StatusBadRequest,
	"roles.err_invalid_subject": http.StatusBadRequest,

	"students.err_not_found":            http.StatusNotFound,
	"students.err_invalid_id":           http.StatusBadRequest,
	"students.err_invalid_name":         http.StatusBadRequest,
	"students.err_invalid_class":        http.StatusBadRequest,
	"students.err_photo_too_large":      http.StatusRequestEntityTooLarge,
	"students.err_invalid_document":     http.StatusBadRequest,
	"students.err_invalid_birth":        http.StatusBadRequest,
	"students.err_invalid_blood_type":   http.StatusBadRequest,
	"students.err_invalid_email":        http.StatusBadRequest,
	"students.err_invalid_guardian":     http.StatusBadRequest,
	"students.err_invalid_sibling":      http.StatusBadRequest,
	"students.err_invalid_health":       http.StatusBadRequest,
	"students.err_invalid_repeat_count": http.StatusBadRequest,
	"students.err_already_graduated":    http.StatusBadRequest,

	"attendance.err_unknown_reason":  http.StatusBadRequest,
	"attendance.err_invalid_student": http.StatusBadRequest,
	"attendance.err_invalid_class":   http.StatusBadRequest,
	"attendance.err_invalid_date":    http.StatusBadRequest,
	"attendance.err_not_found":       http.StatusNotFound,

	"incidents.err_unknown_severity":  http.StatusBadRequest,
	"incidents.err_invalid_student":   http.StatusBadRequest,
	"incidents.err_invalid_class":     http.StatusBadRequest,
	"incidents.err_invalid_date":      http.StatusBadRequest,
	"incidents.err_empty_description": http.StatusBadRequest,
	"incidents.err_not_found":         http.StatusNotFound,

	"warnings.err_unknown_gravity":   http.StatusBadRequest,
	"warnings.err_invalid_student":   http.StatusBadRequest,
	"warnings.err_invalid_class":     http.StatusBadRequest,
	"warnings.err_invalid_teacher":   http.StatusBadRequest,
	"warnings.err_invalid_date":      http.StatusBadRequest,
	"warnings.err_empty_title":       http.StatusBadRequest,
	"warnings.err_empty_description": http.StatusBadRequest,
	"warnings.err_invalid_snapshot":  http.StatusBadRequest,
	"warnings.err_invalid_group":     http.StatusBadRequest,
	"warnings.err_empty_batch":       http.StatusBadRequest,
	"warnings.err_not_found":         http.StatusNotFound,

	"teachers.err_not_found":        http.StatusNotFound,
	"teachers.err_invalid_id":       http.StatusBadRequest,
	"teachers.err_invalid_name":     http.StatusBadRequest,
	"teachers.err_invalid_document": http.StatusBadRequest,
	"teachers.err_invalid_birth":    http.StatusBadRequest,
	"teachers.err_invalid_email":    http.StatusBadRequest,

	"subjects.err_not_found":            http.StatusNotFound,
	"subjects.err_assignment_not_found": http.StatusNotFound,
	"subjects.err_invalid_id":           http.StatusBadRequest,
	"subjects.err_invalid_name":         http.StatusBadRequest,
	"subjects.err_invalid_teacher":      http.StatusBadRequest,
	"subjects.err_invalid_subject":      http.StatusBadRequest,
	"subjects.err_invalid_date":         http.StatusBadRequest,
	"subjects.err_invalid_end":          http.StatusBadRequest,
	"subjects.err_duplicate_subject":    http.StatusBadRequest,
	"subjects.err_already_assigned":     http.StatusBadRequest,
	"subjects.err_already_ended":        http.StatusBadRequest,
	"subjects.err_has_assignments":      http.StatusBadRequest,

	"schoolyears.err_not_found":       http.StatusNotFound,
	"schoolyears.err_invalid_id":      http.StatusBadRequest,
	"schoolyears.err_invalid_year":    http.StatusBadRequest,
	"schoolyears.err_invalid_periods": http.StatusBadRequest,
	"schoolyears.err_invalid_dates":   http.StatusBadRequest,
	"schoolyears.err_invalid_holiday": http.StatusBadRequest,
	"schoolyears.err_duplicate_year":  http.StatusBadRequest,
	"schoolyears.err_has_classes":     http.StatusBadRequest,

	"classes.err_not_found":           http.StatusNotFound,
	"classes.err_invalid_id":          http.StatusBadRequest,
	"classes.err_invalid_school_year": http.StatusBadRequest,
	"classes.err_invalid_grade":       http.StatusBadRequest,
	"classes.err_invalid_group":       http.StatusBadRequest,
	"classes.err_duplicate_class":     http.StatusBadRequest,
	"classes.err_has_enrollments":     http.StatusBadRequest,
	"classes.err_has_sessions":        http.StatusBadRequest,

	"enrollments.err_invalid_id":           http.StatusBadRequest,
	"enrollments.err_invalid_student":      http.StatusBadRequest,
	"enrollments.err_invalid_class_group":  http.StatusBadRequest,
	"enrollments.err_duplicate_enrollment": http.StatusBadRequest,
	"enrollments.err_invalid_decision":     http.StatusBadRequest,
	"enrollments.err_invalid_promotion":    http.StatusBadRequest,
	"enrollments.err_invalid_date":         http.StatusBadRequest,
	"enrollments.err_invalid_actor":        http.StatusBadRequest,

	"schedules.err_not_found":            http.StatusNotFound,
	"schedules.err_invalid_id":           http.StatusBadRequest,
	"schedules.err_invalid_class_group":  http.StatusBadRequest,
	"schedules.err_invalid_teacher":      http.StatusBadRequest,
	"schedules.err_invalid_subject":      http.StatusBadRequest,
	"schedules.err_invalid_weekday":      http.StatusBadRequest,
	"schedules.err_invalid_time":         http.StatusBadRequest,
	"schedules.err_teacher_conflict":     http.StatusBadRequest,
	"schedules.err_class_conflict":       http.StatusBadRequest,
	"schedules.err_not_teaching_subject": http.StatusBadRequest,

	"sessions.err_not_found":           http.StatusNotFound,
	"sessions.err_invalid_id":          http.StatusBadRequest,
	"sessions.err_invalid_class_group": http.StatusBadRequest,
	"sessions.err_invalid_teacher":     http.StatusBadRequest,
	"sessions.err_invalid_subject":     http.StatusBadRequest,
	"sessions.err_invalid_date":        http.StatusBadRequest,
	"sessions.err_invalid_period":      http.StatusBadRequest,
	"sessions.err_invalid_class_label": http.StatusBadRequest,
	"sessions.err_invalid_school_year": http.StatusBadRequest,
	"sessions.err_empty_roster":        http.StatusBadRequest,
	"sessions.err_invalid_roster":      http.StatusBadRequest,
	"sessions.err_duplicate_roster":    http.StatusBadRequest,
	"sessions.err_unknown_mark":        http.StatusBadRequest,
	"sessions.err_invalid_student":     http.StatusBadRequest,
	"sessions.err_not_enrolled":        http.StatusBadRequest,
	"sessions.err_no_change":           http.StatusBadRequest,
	"sessions.err_invalid_actor":       http.StatusBadRequest,
}

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a localized error envelope for err. Domain errors should
// implement the i18n Coded contract so their message key is stable.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	code := "http.err_internal"
	if key, ok := i18n.MessageKey(err); ok {
		code = key
	}

	tr, _ := TranslatorFrom(r.Context())
	message := code
	if tr != nil {
		message = tr.Translate(code, nil)
	}

	status := statusByCode[code]
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
