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
