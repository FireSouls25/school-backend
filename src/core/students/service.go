package students

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies profile policy and persists students through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates the profile, assigns a fresh UUID and persists it.
// The ID field of st is ignored.
func (s *Service) Create(ctx context.Context, st Student) (Student, error) {
	st.ID = uuid.NewString()
	st = normalize(st)
	if err := validate(st); err != nil {
		return Student{}, err
	}
	return s.store.Create(ctx, st)
}

// ByID returns the student with the given id.
func (s *Service) ByID(ctx context.Context, id string) (Student, error) {
	if err := validateID(id); err != nil {
		return Student{}, err
	}
	return s.store.ByID(ctx, id)
}

// ListByClass returns every student in classID ordered alphabetically.
func (s *Service) ListByClass(ctx context.Context, classID string) ([]Student, error) {
	if strings.TrimSpace(classID) == "" {
		return nil, ErrInvalidClass
	}
	return s.store.ListByClass(ctx, classID)
}

// Update replaces the profile of an existing student.
func (s *Service) Update(ctx context.Context, st Student) (Student, error) {
	st.ID = strings.TrimSpace(st.ID)
	st = normalize(st)
	if err := validate(st); err != nil {
		return Student{}, err
	}
	return s.store.Update(ctx, st)
}

// Delete removes a student and, by store contract, its dependent records.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// PutPhoto validates size limits and stores the student photo.
func (s *Service) PutPhoto(ctx context.Context, studentID string, photo Photo) error {
	if err := validateID(studentID); err != nil {
		return err
	}
	if len(photo.Data) > MaxPhotoSize {
		return ErrPhotoTooLarge
	}
	return s.store.PutPhoto(ctx, studentID, photo)
}

// Photo returns the stored photo of a student.
func (s *Service) Photo(ctx context.Context, studentID string) (Photo, error) {
	if err := validateID(studentID); err != nil {
		return Photo{}, err
	}
	return s.store.Photo(ctx, studentID)
}

// normalize trims every free-text field and normalizes the blood type
// and email so validation and storage see a canonical form.
func normalize(st Student) Student {
	trim := strings.TrimSpace
	st.Names = trim(st.Names)
	st.Surnames = trim(st.Surnames)
	st.ClassID = trim(st.ClassID)
	st.DocumentID = trim(st.DocumentID)
	st.Phone = trim(st.Phone)
	st.Address = trim(st.Address)
	st.Birthplace = trim(st.Birthplace)
	st.Email = strings.ToLower(trim(st.Email))
	if v, ok := NormalizeBloodType(st.BloodType); ok {
		st.BloodType = v
	} else {
		st.BloodType = trim(st.BloodType)
	}
	st.PreviousSchool = trim(st.PreviousSchool)
	st.TransferReason = trim(st.TransferReason)
	st.Mother = normalizeGuardian(st.Mother)
	st.Father = normalizeGuardian(st.Father)
	st.Caregiver = normalizeGuardian(st.Caregiver)
	st.LivesWith = trim(st.LivesWith)
	for i := range st.Siblings {
		st.Siblings[i].Name = trim(st.Siblings[i].Name)
		st.Siblings[i].ClassID = trim(st.Siblings[i].ClassID)
	}
	st.MedicalReport = trim(st.MedicalReport)
	st.DiversityCondition = trim(st.DiversityCondition)
	st.Vision.Detail = trim(st.Vision.Detail)
	st.Hearing.Detail = trim(st.Hearing.Detail)
	st.SpecialistReport.Detail = trim(st.SpecialistReport.Detail)
	return st
}

func normalizeGuardian(g Guardian) Guardian {
	trim := strings.TrimSpace
	g.Names = trim(g.Names)
	g.DocumentID = trim(g.DocumentID)
	g.Phone = trim(g.Phone)
	g.Occupation = trim(g.Occupation)
	g.Address = trim(g.Address)
	return g
}

func validate(st Student) error {
	if st.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(st.ID); err != nil {
		return ErrInvalidID
	}
	if st.Names == "" || st.Surnames == "" {
		return ErrInvalidName
	}
	if st.ClassID == "" {
		return ErrInvalidClass
	}
	if st.DocumentID == "" {
		return ErrInvalidDocument
	}
	if !st.Birthdate.IsZero() && st.Birthdate.After(time.Now()) {
		return ErrInvalidBirth
	}
	if st.Email != "" {
		if _, err := mail.ParseAddress(st.Email); err != nil {
			return ErrInvalidEmail
		}
	}
	if st.BloodType != "" {
		if _, ok := NormalizeBloodType(st.BloodType); !ok {
			return ErrInvalidBloodType
		}
	}
	if st.RepeatCount < 0 {
		return ErrInvalidRepeatCount
	}
	if st.Vision.Has && st.Vision.Detail == "" ||
		st.Hearing.Has && st.Hearing.Detail == "" ||
		st.SpecialistReport.Has && st.SpecialistReport.Detail == "" {
		return ErrInvalidHealth
	}
	// Mother and father are optional, but a named guardian needs no
	// further mandatory fields. The acudiente (caregiver) is the
	// notification contact, so its name is required.
	if (!st.Mother.IsEmpty() && st.Mother.Names == "") ||
		(!st.Father.IsEmpty() && st.Father.Names == "") {
		return ErrInvalidGuardian
	}
	if st.Caregiver.Names == "" {
		return ErrInvalidGuardian
	}
	for _, sib := range st.Siblings {
		if sib.Name == "" || sib.ClassID == "" {
			return ErrInvalidSibling
		}
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}
