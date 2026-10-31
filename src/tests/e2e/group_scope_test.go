// Alcance docente-por-salón y contrato de permisos.
//
// Un docente solo opera en los salones de su horario: la URL de la lista
// de otro docente no muestra ni cambia nada (403). El admin conserva
// alcance total y /v1/me expone los permisos efectivos que el frontend
// usa para mostrar u ocultar acciones.
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func mustGroupsOf(t *testing.T, h http.Handler, token string) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for w := 0; w <= 6; w++ {
		code, raw := flowDo(t, h, http.MethodGet, fmt.Sprintf("/v1/teachers/me/schedule?weekday=%d", w), token, "")
		if code != http.StatusOK {
			t.Fatalf("schedule w=%d = %d, want 200: %s", w, code, raw)
		}
		var entries []map[string]any
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("schedule w=%d inválido: %s", w, raw)
		}
		for _, e := range entries {
			if gid, ok := e["GroupID"].(string); ok && gid != "" && !seen[gid] {
				seen[gid] = true
				out = append(out, gid)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("docente sin salones en el seed")
	}
	return out
}

func TestTeacherGroupScopeContract(t *testing.T) {
	h := boot(t)
	aToken := flowToken(t, h, "admin", "admin123*")
	dToken := flowToken(t, h, "docente", "docente123*")

	// Permisos efectivos en /v1/me: el contrato que lee el frontend.
	perms := func(token string) []string {
		t.Helper()
		code, raw := flowDo(t, h, http.MethodGet, "/v1/me", token, "")
		if code != http.StatusOK {
			t.Fatalf("me = %d, want 200: %s", code, raw)
		}
		var me map[string]any
		if err := json.Unmarshal(raw, &me); err != nil {
			t.Fatalf("me inválido: %s", raw)
		}
		var out []string
		if list, ok := me["permissions"].([]any); ok {
			for _, p := range list {
				if s, ok := p.(string); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	has := func(list []string, want string) bool {
		for _, p := range list {
			if p == want {
				return true
			}
		}
		return false
	}
	dPerms := perms(dToken)
	for _, want := range []string{"record-attendance", "record-incidents", "view-class-statistics"} {
		if !has(dPerms, want) {
			t.Fatalf("docente sin permiso %q: %v", want, dPerms)
		}
	}
	if has(dPerms, "manage-users") {
		t.Fatalf("docente no debe tener manage-users: %v", dPerms)
	}
	if aPerms := perms(aToken); !has(aPerms, "manage-users") {
		t.Fatalf("admin sin manage-users: %v", aPerms)
	}

	// /v1/today: día calendario del servidor para toda la app.
	code, raw := flowDo(t, h, http.MethodGet, "/v1/today", dToken, "")
	if code != http.StatusOK {
		t.Fatalf("today = %d, want 200: %s", code, raw)
	}
	var today map[string]any
	if err := json.Unmarshal(raw, &today); err != nil {
		t.Fatalf("today inválido: %s", raw)
	}
	date, _ := today["date"].(string)
	if len(date) != 10 || date[4] != '-' || date[7] != '-' {
		t.Fatalf("today.date sin forma YYYY-MM-DD: %s", raw)
	}
	if wd, ok := today["weekday"].(float64); !ok || wd < 1 || wd > 7 {
		t.Fatalf("today.weekday fuera de 1..7: %s", raw)
	}

	docGroups := mustGroupsOf(t, h, dToken)
	alien := docGroups[0]

	// Segundo docente: perfil real del seed con cuenta recién creada.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/me", dToken, "")
	var meDoc map[string]any
	_ = json.Unmarshal(raw, &meDoc)
	ownTeacher, _ := meDoc["teacher"].(map[string]any)["id"].(string)
	code, raw = flowDo(t, h, http.MethodGet, "/v1/teachers", aToken, "")
	if code != http.StatusOK {
		t.Fatalf("teachers = %d, want 200: %s", code, raw)
	}
	var profiles []map[string]any
	if err := json.Unmarshal(raw, &profiles); err != nil || len(profiles) < 2 {
		t.Fatalf("sin perfiles para el segundo docente: %s", raw)
	}
	other := ""
	for _, p := range profiles {
		if id, _ := p["ID"].(string); id != "" && id != ownTeacher {
			other = id
			break
		}
	}
	if other == "" {
		t.Fatal("seed sin segundo perfil docente")
	}
	code, raw = flowDo(t, h, http.MethodPost, "/v1/users", aToken,
		`{"username":"profe2","email":"","password":"Profe2-12345x","role":"teacher","teacherID":"`+other+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear profe2 = %d, want 201: %s", code, raw)
	}
	p2Token := flowToken(t, h, "profe2", "Profe2-12345x")

	// Sin horario asignado no toca nada del otro docente.
	for _, tc := range []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"roster ajeno", http.MethodGet, "/v1/classes/" + alien + "/roster", ""},
		{"open ajeno", http.MethodPost, "/v1/sessions/open", `{"classGroupID":"` + alien + `"}`},
		{"batch ajeno", http.MethodPost, "/v1/warnings/batch", `{"studentIDs":["00000000-0000-4000-8000-000000000000"],"classID":"` + alien + `","title":"X","gravity":"mild","description":"Y"}`},
	} {
		c, r := flowDo(t, h, tc.method, tc.target, p2Token, tc.body)
		if c != http.StatusForbidden {
			t.Fatalf("%s = %d, want 403: %s", tc.name, c, r)
		}
	}

	// Al asignarle horario en otro salón sí opera ahí…
	code, raw = flowDo(t, h, http.MethodGet, "/v1/subjects", aToken, "")
	if code != http.StatusOK {
		t.Fatalf("subjects = %d, want 200: %s", code, raw)
	}
	var subjects []map[string]any
	if err := json.Unmarshal(raw, &subjects); err != nil || len(subjects) == 0 {
		t.Fatalf("sin materias en el seed: %s", raw)
	}
	subID, _ := subjects[0]["ID"].(string)
	// …eligiendo un salón y bloque libres de choques (sábado).
	code, raw = flowDo(t, h, http.MethodGet, "/v1/classes?year=2026", aToken, "")
	var classes []map[string]any
	_ = json.Unmarshal(raw, &classes)
	own := ""
	for _, c := range classes {
		if gid, _ := c["ID"].(string); gid != "" && gid != alien {
			own = gid
			break
		}
	}
	if own == "" || subID == "" {
		t.Fatalf("sin salón libre para profe2: %s", raw)
	}
	schedBody := fmt.Sprintf(`{"classGroupID":%q,"teacherID":%q,"subjectID":%q,"weekday":6,"start":"07:00","end":"08:00"}`, own, other, subID)
	code, raw = flowDo(t, h, http.MethodPost, "/v1/schedule", aToken, schedBody)
	if code != http.StatusCreated {
		t.Fatalf("horario profe2 = %d, want 201: %s", code, raw)
	}
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/open", p2Token, `{"classGroupID":"`+own+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open propio = %d, want 201: %s", code, raw)
	}
	// …pero el salón ajeno sigue vedado: ni lectura del historial.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/sessions/history?classGroupID="+alien, p2Token, "")
	if code != http.StatusForbidden {
		t.Fatalf("history ajeno = %d, want 403: %s", code, raw)
	}
	// El admin conserva alcance total.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/classes/"+alien+"/roster", aToken, "")
	if code != http.StatusOK {
		t.Fatalf("roster admin = %d, want 200", code)
	}
	var roster []map[string]any
	if err := json.Unmarshal(raw, &roster); err != nil || len(roster) == 0 {
		t.Fatalf("roster admin vacío: %s", raw)
	}
	for _, r := range roster {
		if slug, _ := r["Slug"].(string); slug == "" {
			t.Fatalf("nómina sin Slug: %v", r)
		}
	}
}

// Filas viejas traen ClassID como etiqueta ("7-1") en vez de UUID: el
// alcance se resuelve por matrículas vivas, no por ese campo.
func TestTeacherScopeLegacyLabel(t *testing.T) {
	h := boot(t)
	aToken := flowToken(t, h, "admin", "admin123*")
	dToken := flowToken(t, h, "docente", "docente123*")
	groups := mustGroupsOf(t, h, dToken)
	own := groups[0]

	createStudent := func(doc, classID string) string {
		t.Helper()
		code, raw := flowDo(t, h, http.MethodPost, "/v1/students", aToken,
			`{"names":"Legacy","surnames":"Caso `+doc+`","documentID":"`+doc+`","classID":"`+classID+`","caregiver":{"names":"Acudiente `+doc+`"}}`)
		if code != http.StatusCreated {
			t.Fatalf("crear estudiante = %d, want 201: %s", code, raw)
		}
		var st map[string]any
		if err := json.Unmarshal(raw, &st); err != nil || st["ID"] == nil {
			t.Fatalf("estudiante sin ID: %s", raw)
		}
		return st["ID"].(string)
	}

	labeled := createStudent("90000001", "7-1-etiqueta")
	code, raw := flowDo(t, h, http.MethodPost, "/v1/enrollments", aToken,
		`{"studentID":"`+labeled+`","classGroupID":"`+own+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("matricular = %d, want 201: %s", code, raw)
	}
	code, raw = flowDo(t, h, http.MethodGet, "/v1/statistics/student/"+labeled, dToken, "")
	if code != http.StatusOK {
		t.Fatalf("reporte con etiqueta vieja = %d, want 200: %s", code, raw)
	}

	orphan := createStudent("90000002", "9-9-fantasma")
	code, raw = flowDo(t, h, http.MethodGet, "/v1/statistics/student/"+orphan, dToken, "")
	if code != http.StatusForbidden {
		t.Fatalf("reporte huérfano = %d, want 403: %s", code, raw)
	}
}

// Las URLs solo muestran slugs opacos: únicos por entidad, resolvibles
// al id interno, y presentes en los listados que enlazan.
func TestSlugContract(t *testing.T) {
	h := boot(t)
	aToken := flowToken(t, h, "admin", "admin123*")
	dToken := flowToken(t, h, "docente", "docente123*")

	// El docente ve slugs (no UUIDs) en su horario.
	code, raw := flowDo(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=1", dToken, "")
	if code != http.StatusOK {
		t.Fatalf("schedule = %d, want 200: %s", code, raw)
	}
	if !strings.Contains(string(raw), "GroupSlug") {
		t.Fatalf("horario sin GroupSlug: %s", raw)
	}
	var sched []map[string]any
	if err := json.Unmarshal(raw, &sched); err != nil || len(sched) == 0 {
		t.Fatalf("horario vacío: %s", raw)
	}
	seen := map[string]bool{}
	for _, e := range sched {
		slug, _ := e["GroupSlug"].(string)
		gid, _ := e["GroupID"].(string)
		if slug == "" || slug == gid {
			t.Fatalf("GroupSlug ausente o igual al UUID: %v", e)
		}
		if seen[slug] {
			t.Fatalf("slug de grupo duplicado: %s", slug)
		}
		seen[slug] = true
	}

	// El resolvedor traduce slug -> id interno.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/s/"+firstSlug(t, sched), dToken, "")
	if code != http.StatusOK {
		t.Fatalf("resolver = %d, want 200: %s", code, raw)
	}
	var resolved map[string]any
	if err := json.Unmarshal(raw, &resolved); err != nil {
		t.Fatalf("resolver inválido: %s", raw)
	}
	if resolved["kind"] != "class" || resolved["id"] == nil || resolved["id"] == "" {
		t.Fatalf("resolver sin kind/id: %s", raw)
	}
	code, _ = flowDo(t, h, http.MethodGet, "/v1/s/noexiste123", dToken, "")
	if code != http.StatusNotFound {
		t.Fatalf("slug falso = %d, want 404", code)
	}

	// El reporte trae Slug por estudiante para los enlaces.
	groups := mustGroupsOf(t, h, dToken)
	code, raw = flowDo(t, h, http.MethodGet, "/v1/statistics/class/"+groups[0], aToken, "")
	if code != http.StatusOK {
		t.Fatalf("class report = %d, want 200: %s", code, raw)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report inválido: %s", raw)
	}
	students, _ := report["Students"].([]any)
	if len(students) == 0 {
		t.Fatalf("report sin estudiantes: %s", raw)
	}
	for _, s := range students {
		m, _ := s.(map[string]any)
		if slug, _ := m["Slug"].(string); slug == "" {
			t.Fatalf("estudiante sin Slug en reporte: %v", m)
		}
	}
}

func firstSlug(t *testing.T, sched []map[string]any) string {
	t.Helper()
	for _, e := range sched {
		if slug, ok := e["GroupSlug"].(string); ok && slug != "" {
			return slug
		}
	}
	t.Fatal("sin GroupSlug en el horario")
	return ""
}
