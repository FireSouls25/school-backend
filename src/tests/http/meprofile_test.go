package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/roles"
	"grade/src/core/teachers"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func meProfileStack(t *testing.T) (http.Handler, *users.Service, *teachers.Service, *roles.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Users:            usersSvc,
		Teachers:         teachersSvc,
	}, newI18n(t)), usersSvc, teachersSvc, roleSvc
}

// seedLinkedTeacher creates a teacher profile, an account linked to it
// and returns both ids plus the account username.
func seedLinkedTeacher(t *testing.T, usersSvc *users.Service, teachersSvc *teachers.Service, roleSvc *roles.Service) (string, string) {
	t.Helper()
	ctx := context.Background()
	tc, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "1122334455",
		Phone: "3001234567", Email: "carlos@colegio.edu",
		Address: "Calle 10 # 5-20", Birthplace: "Medellín",
		MedicalConditions: "Ninguna",
		Birthdate: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		Sex:       teachers.SexMale,
	})
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	account, err := usersSvc.Create(ctx, "carlos.mendoza", "carlos@colegio.edu", "docente123*")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := usersSvc.LinkTeacher(ctx, account.ID, tc.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := roleSvc.Assign(ctx, account.ID, roles.RoleTeacher); err != nil {
		t.Fatalf("grant teacher: %v", err)
	}
	return account.ID, tc.ID
}

func TestMeCarriesAccountAndTeacherProfile(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, teacherID := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	rec := doRequest(t, h, http.MethodGet, "/v1/me", accountID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Subject   string   `json:"subject"`
		Roles     []string `json:"roles"`
		Home      string   `json:"home"`
		Username  string   `json:"username"`
		Email     string   `json:"email"`
		Active    bool     `json:"active"`
		CreatedAt string   `json:"createdAt"`
		TeacherID string   `json:"teacherID"`
		Teacher   struct {
			ID                string `json:"id"`
			Names             string `json:"names"`
			Surnames          string `json:"surnames"`
			DocumentID        string `json:"documentID"`
			Phone             string `json:"phone"`
			Email             string `json:"email"`
			Address           string `json:"address"`
			Birthplace        string `json:"birthplace"`
			MedicalConditions string `json:"medicalConditions"`
			Birthdate         string `json:"birthdate"`
			Sex               string `json:"sex"`
		} `json:"teacher"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body.Subject != accountID || body.Home != "/docente" {
		t.Errorf("me = %+v, want subject %q home /docente", body, accountID)
	}
	if body.Username != "carlos.mendoza" || body.Email != "carlos@colegio.edu" || !body.Active {
		t.Errorf("account = username %q email %q active %v", body.Username, body.Email, body.Active)
	}
	if body.CreatedAt == "" {
		t.Error("createdAt is empty")
	}
	if body.TeacherID != teacherID {
		t.Errorf("teacherID = %q, want %q", body.TeacherID, teacherID)
	}
	if body.Teacher.ID != teacherID || body.Teacher.Names != "Carlos" || body.Teacher.Surnames != "Mendoza" {
		t.Errorf("teacher = %+v, want linked profile", body.Teacher)
	}
	if body.Teacher.DocumentID != "1122334455" || body.Teacher.Phone != "3001234567" {
		t.Errorf("teacher contact = %+v", body.Teacher)
	}
	if body.Teacher.Birthdate != "1990-05-20" || body.Teacher.Sex != "male" {
		t.Errorf("teacher birth/sex = %q/%q", body.Teacher.Birthdate, body.Teacher.Sex)
	}
	if body.Teacher.Address != "Calle 10 # 5-20" || body.Teacher.Birthplace != "Medellín" || body.Teacher.MedicalConditions != "Ninguna" {
		t.Errorf("teacher ficha = %+v, want full own record", body.Teacher)
	}
}

func TestMeWithoutAccountStaysNavigationOnly(t *testing.T) {
	h, _, _, _ := meProfileStack(t)

	// Dev identity: no users row, still 200 with roles and home.
	rec := doRequest(t, h, http.MethodGet, "/v1/me", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		TeacherID string `json:"teacherID"`
		Teacher   any    `json:"teacher"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body.Username != "" || body.Email != "" || body.TeacherID != "" || body.Teacher != nil {
		t.Errorf("dev identity leaked account data: %+v", body)
	}
}

func TestMyTeacherPhotoSelfService(t *testing.T) {
	h, usersSvc, teachersSvc, roleSvc := meProfileStack(t)
	accountID, teacherID := seedLinkedTeacher(t, usersSvc, teachersSvc, roleSvc)

	// No photo yet: honest 404.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/me/photo", accountID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing photo status = %d, want 404", rec.Code)
	}

	// Upload through the link, read back, clear.
	payload := `{"data":"` + base64.StdEncoding.EncodeToString(tinyPNG) + `"}`
	rec = doRequest(t, h, http.MethodPut, "/v1/teachers/me/photo", accountID, payload)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("upload status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/photo", accountID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	var photo map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &photo); err != nil {
		t.Fatalf("decode photo: %v", err)
	}
	if photo["contentType"] != "image/png" {
		t.Errorf("contentType = %q, want image/png", photo["contentType"])
	}

	// The photo really landed on the linked profile.
	stored, err := teachersSvc.Photo(context.Background(), teacherID)
	if err != nil || len(stored.Data) == 0 {
		t.Fatalf("stored photo = %v, %v", stored.Data, err)
	}

	// Clearing goes back to the honest 404.
	rec = doRequest(t, h, http.MethodPut, "/v1/teachers/me/photo", accountID, `{"data":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d, want 204", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/photo", accountID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after clear status = %d, want 404", rec.Code)
	}
}

func TestMyTeacherPhotoDeniedWithoutLink(t *testing.T) {
	h, _, _, _ := meProfileStack(t)

	// Unknown subject (valid UUID, no account, no teacher profile):
	// 403, no data leak.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/me/photo", unknownSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unknown get status = %d, want 403", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPut, "/v1/teachers/me/photo", unknownSubject, `{"data":""}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unknown put status = %d, want 403", rec.Code)
	}
}
