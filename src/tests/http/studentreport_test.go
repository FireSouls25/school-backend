package httpapi_test

import (
	"net/http"
	"testing"
)

// The student reads their own report but never another student's.
func TestStudentReadsOwnReportOnly(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+studentSubject, studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own report status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+otherStudent, studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other report status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+studentSubject, otherStudent, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("peer reading report status = %d, want 403", rec.Code)
	}
}
