package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"

	"grade/src/core/roles"
	httpapi "grade/src/platform/http"
)

func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	roleSvc := roles.NewService(roles.NewMemoryStore())
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
	}, newI18n(t))

	// Unknown routes log method, path and status.
	rec := doRequest(t, h, http.MethodGet, "/v1/noexiste", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	out := buf.String()
	for _, want := range []string{"method=GET", "path=/v1/noexiste", "status=404"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Fatalf("log = %q, want it to contain %q", out, want)
		}
	}

	// /healthz stays silent: probes must not drown real traffic.
	buf.Reset()
	rec = doRequest(t, h, http.MethodGet, "/healthz", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
	if buf.Len() != 0 {
		t.Fatalf("healthz logged %q, want silence", buf.String())
	}
}
