package config_test

import (
	"testing"

	"grade/src/platform/config"
)

func TestNotificationEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("WHATSAPP_TOKEN", "watest")
	t.Setenv("WHATSAPP_PHONE_ID", "phone-1")
	t.Setenv("WHATSAPP_API_BASE", "https://proxy.local/")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("RESEND_FROM", " Colegio <no-reply@updates.fractalschool.app> ")
	t.Setenv("PUBLIC_URL", "https://fractalschool.app/")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.WhatsappToken != "watest" || cfg.WhatsappPhoneID != "phone-1" {
		t.Errorf("whatsapp = %q/%q", cfg.WhatsappToken, cfg.WhatsappPhoneID)
	}
	if cfg.WhatsappAPIBase != "https://proxy.local/" {
		t.Errorf("api base = %q", cfg.WhatsappAPIBase)
	}
	if cfg.ResendFrom != "Colegio <no-reply@updates.fractalschool.app>" {
		t.Errorf("resend from = %q, want trimmed", cfg.ResendFrom)
	}
	if cfg.PublicURL != "https://fractalschool.app" {
		t.Errorf("public URL = %q, want no trailing slash", cfg.PublicURL)
	}
}
