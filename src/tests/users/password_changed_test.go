package users_test

import (
	"context"
	"testing"
)

func TestPasswordChangedFlag(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	u, err := svc.Create(ctx, "admin", "", "admin123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.ByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.PasswordChanged {
		t.Error("new account PasswordChanged = true, want false (initial password)")
	}
	if err := svc.ChangePassword(ctx, u.ID, "admin123*", "nueva456*"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	got, err = svc.ByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if !got.PasswordChanged {
		t.Error("after ChangePassword PasswordChanged = false, want true")
	}
}
