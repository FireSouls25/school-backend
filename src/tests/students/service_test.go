package students_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"grade/src/core/students"
)

func newService() *students.Service {
	return students.NewService(students.NewMemoryStore())
}

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	t.Run("missing names", func(t *testing.T) {
		_, err := svc.Create(ctx, "  ", "Gómez", "9-1")
		if !errors.Is(err, students.ErrInvalidName) {
			t.Errorf("Create error = %v, want ErrInvalidName", err)
		}
	})

	t.Run("missing surnames", func(t *testing.T) {
		_, err := svc.Create(ctx, "Ana", "", "9-1")
		if !errors.Is(err, students.ErrInvalidName) {
			t.Errorf("Create error = %v, want ErrInvalidName", err)
		}
	})

	t.Run("missing class", func(t *testing.T) {
		_, err := svc.Create(ctx, "Ana", "Gómez", " ")
		if !errors.Is(err, students.ErrInvalidClass) {
			t.Errorf("Create error = %v, want ErrInvalidClass", err)
		}
	})
}

func TestServiceCreateAssignsUUID(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Create(ctx, "Ana", "Gómez", "9-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := svc.Create(ctx, "Beto", "Ruiz", "9-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" || a.ID == b.ID {
		t.Errorf("IDs not unique: %q vs %q", a.ID, b.ID)
	}
	want := students.Student{ID: a.ID, Names: "Ana", Surnames: "Gómez", ClassID: "9-1"}
	got, err := svc.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got != want {
		t.Errorf("ByID = %+v, want %+v", got, want)
	}
}

func TestFullNameSurnamesFirst(t *testing.T) {
	st := students.Student{Names: "Ana María", Surnames: "Gómez Ruiz"}
	if got, want := st.FullName(), "Gómez Ruiz Ana María"; got != want {
		t.Errorf("FullName = %q, want %q", got, want)
	}
}

func TestListByClassSortedAlphabetically(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	for _, st := range []struct{ names, surnames string }{
		{"Carla", "Zapata"},
		{"Ana", "gomez"}, // case-insensitive ordering
		{"Beto", "López"},
		{"Xavier", "Álvarez"}, // accent-insensitive expectation falls on store
	} {
		if _, err := svc.Create(ctx, st.names, st.surnames, "9-1"); err != nil {
			t.Fatalf("Create %s %s: %v", st.surnames, st.names, err)
		}
	}
	if _, err := svc.Create(ctx, "Otto", "Otro", "10-2"); err != nil {
		t.Fatalf("Create other class: %v", err)
	}

	list, err := svc.ListByClass(ctx, "9-1")
	if err != nil {
		t.Fatalf("ListByClass: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("len(list) = %d, want 4", len(list))
	}
	for i := 1; i < len(list); i++ {
		prev := strings.ToLower(list[i-1].FullName())
		cur := strings.ToLower(list[i].FullName())
		if prev > cur {
			t.Errorf("list not alphabetical: %q before %q", prev, cur)
		}
	}
}

func TestUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	st, err := svc.Create(ctx, "Ana", "Gómez", "9-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	upd, err := svc.Update(ctx, st.ID, "Ana Lucía", "Gómez", "9-2")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.ClassID != "9-2" || upd.Names != "Ana Lucía" {
		t.Errorf("Update = %+v", upd)
	}

	if _, err := svc.Update(ctx, "00000000-0000-0000-0000-000000000001", "X", "Y", "9-1"); !errors.Is(err, students.ErrNotFound) {
		t.Errorf("Update unknown error = %v, want ErrNotFound", err)
	}

	if err := svc.Delete(ctx, st.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, st.ID); !errors.Is(err, students.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
}

func TestPhotoLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	st, err := svc.Create(ctx, "Ana", "Gómez", "9-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.PutPhoto(ctx, st.ID, students.Photo{Data: []byte{0x89, 'P', 'N', 'G'}}); err != nil {
		t.Fatalf("PutPhoto: %v", err)
	}
	ph, err := svc.Photo(ctx, st.ID)
	if err != nil {
		t.Fatalf("Photo: %v", err)
	}
	if len(ph.Data) != 4 {
		t.Errorf("len(Photo.Data) = %d, want 4", len(ph.Data))
	}

	err = svc.PutPhoto(ctx, st.ID, students.Photo{Data: make([]byte, students.MaxPhotoSize+1)})
	if !errors.Is(err, students.ErrPhotoTooLarge) {
		t.Errorf("PutPhoto oversized error = %v, want ErrPhotoTooLarge", err)
	}

	err = svc.PutPhoto(ctx, "00000000-0000-0000-0000-00000000000f", students.Photo{Data: []byte{1}})
	if !errors.Is(err, students.ErrNotFound) {
		t.Errorf("PutPhoto unknown student error = %v, want ErrNotFound", err)
	}
}
