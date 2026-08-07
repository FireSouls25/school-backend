package roles_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"grade/src/core/roles"
)

func TestMemoryStoreAssignAndRoles(t *testing.T) {
	ctx := context.Background()
	store := roles.NewMemoryStore()

	if err := store.AssignRole(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := store.AssignRole(ctx, "s1", roles.RoleAdmin); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	got, err := store.RolesFor(ctx, "s1")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	want := []roles.Role{roles.RoleAdmin, roles.RoleTeacher}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RolesFor = %v, want %v", got, want)
	}
}

func TestMemoryStoreAssignIdempotent(t *testing.T) {
	ctx := context.Background()
	store := roles.NewMemoryStore()

	if err := store.AssignRole(ctx, "s1", roles.RoleStudent); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := store.AssignRole(ctx, "s1", roles.RoleStudent); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	got, err := store.RolesFor(ctx, "s1")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	if len(got) != 1 || got[0] != roles.RoleStudent {
		t.Errorf("RolesFor = %v, want single RoleStudent", got)
	}
}

func TestMemoryStoreRemove(t *testing.T) {
	ctx := context.Background()
	store := roles.NewMemoryStore()

	if err := store.AssignRole(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if err := store.RemoveRole(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("RemoveRole: %v", err)
	}

	got, err := store.RolesFor(ctx, "s1")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("RolesFor after remove = %v, want empty", got)
	}
}

func TestMemoryStoreRemoveAbsentIsNoOp(t *testing.T) {
	ctx := context.Background()
	store := roles.NewMemoryStore()
	if err := store.RemoveRole(ctx, "s1", roles.RoleStudent); err != nil {
		t.Fatalf("RemoveRole: %v", err)
	}
	got, err := store.RolesFor(ctx, "s1")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("RolesFor = %v, want empty", got)
	}
}

func TestMemoryStoreUnknownSubject(t *testing.T) {
	ctx := context.Background()
	store := roles.NewMemoryStore()

	got, err := store.RolesFor(ctx, "nobody")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("RolesFor unknown = %v, want empty", got)
	}
}

func TestMemoryStoreConcurrentSafety(t *testing.T) {
	store := roles.NewMemoryStore()
	const goroutines = 50
	const rolesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < rolesPerGoroutine; i++ {
				role := roles.RoleAdmin
				if (g+i)%2 == 0 {
					role = roles.RoleStudent
				}
				if err := store.AssignRole(ctx, "s1", role); err != nil {
					t.Errorf("AssignRole: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	got, err := store.RolesFor(context.Background(), "s1")
	if err != nil {
		t.Fatalf("RolesFor: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("RolesFor = %v, want both roles", got)
	}
}
