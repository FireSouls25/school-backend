package students_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grade/src/core/students"
)

func newService() *students.Service {
	return students.NewService(students.NewMemoryStore())
}

// validProfile returns a fully filled, valid student profile.
func validProfile() students.Student {
	return students.Student{
		Names:      "Ana",
		Surnames:   "Gómez",
		ClassID:    "9-1",
		DocumentID: "1234567890",
		Phone:      "3001234567",
		Address:    "Calle 10 # 5-20",
		Birthplace: "Bogotá",
		Birthdate:  time.Date(2012, 4, 15, 0, 0, 0, 0, time.UTC),
		Email:      "ana.gomez@example.com",
		Vision:     students.Condition{Has: true, Detail: "Miopía"},
		Hearing:    students.Condition{Has: false},
		BloodType:  "O+",
		IsNew:      true,
		Mother: students.Guardian{
			Names: "María Gómez", DocumentID: "51999888",
			Phone: "3101112233", Occupation: "Docente", Address: "Calle 10 # 5-20",
		},
		Father: students.Guardian{
			Names: "José Ruiz", DocumentID: "79999777",
			Phone: "3114445566", Occupation: "Comerciante", Address: "Calle 10 # 5-20",
		},
		Caregiver: students.Guardian{
			Names: "María Gómez", DocumentID: "51999888",
			Phone: "3101112233", Occupation: "Docente", Address: "Calle 10 # 5-20",
		},
		LivesWith: "Padres",
		Siblings: []students.Sibling{
			{Name: "Luis Gómez", ClassID: "7-2"},
		},
		MedicalReport:      "Asma leve",
		DiversityCondition: "",
		SpecialistReport:   students.Condition{Has: false},
	}
}

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*students.Student)
		want   error
	}{
		{"missing names", func(s *students.Student) { s.Names = "  " }, students.ErrInvalidName},
		{"missing surnames", func(s *students.Student) { s.Surnames = "" }, students.ErrInvalidName},
		{"missing class", func(s *students.Student) { s.ClassID = " " }, students.ErrInvalidClass},
		{"missing document", func(s *students.Student) { s.DocumentID = "" }, students.ErrInvalidDocument},
		{"missing caregiver", func(s *students.Student) { s.Caregiver = students.Guardian{} }, students.ErrInvalidGuardian},
		{"bad email", func(s *students.Student) { s.Email = "not-an-email" }, students.ErrInvalidEmail},
		{"bad blood type", func(s *students.Student) { s.BloodType = "Z+" }, students.ErrInvalidBloodType},
		{"future birthdate", func(s *students.Student) {
			s.Birthdate = time.Now().Add(24 * time.Hour)
		}, students.ErrInvalidBirth},
		{"vision without detail", func(s *students.Student) {
			s.Vision = students.Condition{Has: true}
		}, students.ErrInvalidHealth},
		{"specialist without detail", func(s *students.Student) {
			s.SpecialistReport = students.Condition{Has: true}
		}, students.ErrInvalidHealth},
		{"negative repeat count", func(s *students.Student) { s.RepeatCount = -1 }, students.ErrInvalidRepeatCount},
		{"sibling without class", func(s *students.Student) {
			s.Siblings = []students.Sibling{{Name: "Luis"}}
		}, students.ErrInvalidSibling},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProfile()
			tc.mutate(&p)
			if _, err := svc.Create(ctx, p); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestServiceCreateAssignsUUID(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Create(ctx, validProfile())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p := validProfile()
	p.Names, p.Surnames, p.DocumentID = "Beto", "Ruiz", "0987654321"
	b, err := svc.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" || a.ID == b.ID {
		t.Errorf("IDs not unique: %q vs %q", a.ID, b.ID)
	}
	got, err := svc.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.Names != "Ana" || got.DocumentID != "1234567890" || got.BloodType != "O+" {
		t.Errorf("ByID = %+v", got)
	}
	if len(got.Siblings) != 1 || got.Siblings[0].Name != "Luis Gómez" {
		t.Errorf("ByID siblings = %+v", got.Siblings)
	}
}

func TestBloodTypeNormalized(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	p := validProfile()
	p.BloodType = "  ab- "
	st, err := svc.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if st.BloodType != "AB-" {
		t.Errorf("BloodType = %q, want %q", st.BloodType, "AB-")
	}
}

func TestFullNameSurnamesFirst(t *testing.T) {
	st := students.Student{Names: "Ana María", Surnames: "Gómez Ruiz"}
	if got, want := st.FullName(), "Gómez Ruiz Ana María"; got != want {
		t.Errorf("FullName = %q, want %q", got, want)
	}
}

func TestAgeAt(t *testing.T) {
	st := students.Student{Birthdate: time.Date(2012, 4, 15, 0, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		ref  time.Time
		want int
		ok   bool
	}{
		{time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), 13, true},
		{time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), 14, true},
		{time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), 14, true},
		{time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), 0, false},
	} {
		got, ok := st.AgeAt(tc.ref)
		if got != tc.want || ok != tc.ok {
			t.Errorf("AgeAt(%v) = %d, %v; want %d, %v", tc.ref, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := (students.Student{}).AgeAt(time.Now()); ok {
		t.Error("AgeAt without birthdate should report unknown")
	}
}

func TestListByClassSortedAlphabetically(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	profiles := []struct{ names, surnames, document string }{
		{"Carla", "Zapata", "1000000001"},
		{"Ana", "gomez", "1000000002"}, // case-insensitive ordering
		{"Beto", "López", "1000000003"},
		{"Xavier", "Álvarez", "1000000004"},
	}
	for _, p := range profiles {
		prof := validProfile()
		prof.Names, prof.Surnames, prof.DocumentID = p.names, p.surnames, p.document
		if _, err := svc.Create(ctx, prof); err != nil {
			t.Fatalf("Create %s %s: %v", p.surnames, p.names, err)
		}
	}
	other := validProfile()
	other.Names, other.Surnames, other.DocumentID, other.ClassID = "Otto", "Otro", "1000000005", "10-2"
	if _, err := svc.Create(ctx, other); err != nil {
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

	st, err := svc.Create(ctx, validProfile())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	st.Names = "Ana Lucía"
	st.ClassID = "9-2"
	st.RepeatCount = 1
	upd, err := svc.Update(ctx, st)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.ClassID != "9-2" || upd.Names != "Ana Lucía" || upd.RepeatCount != 1 {
		t.Errorf("Update = %+v", upd)
	}

	ghost := validProfile()
	ghost.ID = "00000000-0000-0000-0000-000000000001"
	if _, err := svc.Update(ctx, ghost); !errors.Is(err, students.ErrNotFound) {
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

	st, err := svc.Create(ctx, validProfile())
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
