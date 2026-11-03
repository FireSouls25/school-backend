package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grade/src/core/notify"
	"grade/src/platform/whatsapp"
)

func TestSendTemplate(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"mid.1"}]}`))
	}))
	defer srv.Close()

	c, err := whatsapp.NewClient("tok", "phone-1", srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	n := notify.Notification{To: "+573001234567", Template: "falta_hoy", Language: "es", Params: []string{"Ana", "03/11/2026"}}
	if err := c.Send(context.Background(), n); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/phone-1/messages" {
		t.Errorf("path = %q, want /phone-1/messages", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", gotAuth)
	}
	tpl, _ := gotBody["template"].(map[string]any)
	if tpl["name"] != "falta_hoy" {
		t.Errorf("template = %v, want falta_hoy", tpl["name"])
	}
	if gotBody["to"] != "+573001234567" || gotBody["type"] != "template" {
		t.Errorf("body = %v, want template to the normalized phone", gotBody)
	}
}

func TestSendSurfacesMetaErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad template"}}`))
	}))
	defer srv.Close()

	c, err := whatsapp.NewClient("tok", "phone-1", srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = c.Send(context.Background(), notify.Notification{To: "+573001234567", Template: "x"})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("Send error = %v, want the Meta status", err)
	}
}

func TestNewClientRequiresCredentials(t *testing.T) {
	if _, err := whatsapp.NewClient("", "p", ""); err == nil {
		t.Error("empty token ok, want error")
	}
	if _, err := whatsapp.NewClient("t", "", ""); err == nil {
		t.Error("empty phone ok, want error")
	}
}
