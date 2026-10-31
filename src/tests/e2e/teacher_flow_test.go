// Contrato front↔back del flujo docente y su visibilidad en admin.
//
// Replica los cuerpos EXACTOS que envía el SPA (sin periodo al abrir,
// sin snapshots en el batch) y exige el recorrido completo: guardar como
// docente, ver como admin, corregir como admin y volver a ver. Si el
// frontend y el backend se desalinean (campo de más, sobre distinto),
// este test cae antes que el usuario.
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func flowDo(t *testing.T, h http.Handler, method, target, token, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func flowToken(t *testing.T, h http.Handler, identifier, password string) string {
	t.Helper()
	code, raw := flowDo(t, h, http.MethodPost, "/v1/login", "", `{"identifier":"`+identifier+`","password":"`+password+`"}`)
	if code != http.StatusOK {
		t.Fatalf("login %s = %d, want 200: %s", identifier, code, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out["token"] == nil {
		t.Fatalf("login %s sin token: %s", identifier, raw)
	}
	return out["token"].(string)
}

// El docente guarda la lista y el admin la ve: el mismo recorrido que la
// UI, contra la app real con seed demo.
func TestTeacherSaveFlowContract(t *testing.T) {
	h := boot(t)
	dToken := flowToken(t, h, "docente", "docente123*")
	aToken := flowToken(t, h, "admin", "admin123*")

	// Horario del docente: primer bloque con grupo, como Agenda.
	var groupID string
	for w := 0; w <= 6 && groupID == ""; w++ {
		code, raw := flowDo(t, h, http.MethodGet, fmt.Sprintf("/v1/teachers/me/schedule?weekday=%d", w), dToken, "")
		if code != http.StatusOK {
			t.Fatalf("schedule w=%d = %d, want 200: %s", w, code, raw)
		}
		var entries []map[string]any
		if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
			continue
		}
		if gid, ok := entries[0]["GroupID"].(string); ok && gid != "" {
			groupID = gid
		}
	}
	if groupID == "" {
		t.Fatal("seed sin bloques para docente: nada que guardar")
	}

	// Nómina del salón, igual que ClassDetail.
	code, raw := flowDo(t, h, http.MethodGet, "/v1/classes/"+groupID+"/roster", dToken, "")
	if code != http.StatusOK {
		t.Fatalf("roster = %d, want 200: %s", code, raw)
	}
	var roster []map[string]any
	if err := json.Unmarshal(raw, &roster); err != nil || len(roster) == 0 {
		t.Fatalf("roster vacío o inválido: %s", raw)
	}
	studentID, _ := roster[0]["StudentID"].(string)
	if studentID == "" {
		t.Fatalf("roster sin StudentID: %s", raw)
	}

	// Abrir sesión con el cuerpo EXACTO del frontend: solo classGroupID.
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/open", dToken, `{"classGroupID":"`+groupID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open = %d, want 201: %s", code, raw)
	}
	var opened map[string]any
	if err := json.Unmarshal(raw, &opened); err != nil || opened["ID"] == nil {
		t.Fatalf("open sin ID de sesión: %s", raw)
	}
	sessionID := opened["ID"].(string)

	// Marcas con el cuerpo EXACTO del frontend.
	mark := func(sid, m string) {
		t.Helper()
		c, r := flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/marks", dToken,
			`{"studentID":"`+sid+`","mark":"`+m+`","note":""}`)
		if c != http.StatusCreated {
			t.Fatalf("mark %s=%q: %d, want 201: %s", sid, m, c, r)
		}
	}
	mark(studentID, "absence")

	// El detalle muestra la marca que el docente acaba de guardar.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/sessions/"+sessionID, dToken, "")
	if code != http.StatusOK {
		t.Fatalf("session detail = %d, want 200: %s", code, raw)
	}
	var detail map[string]any
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("detail inválido: %s", raw)
	}
	marks, _ := detail["Marks"].(map[string]any)
	if marks[studentID] != "absence" {
		t.Fatalf("detail.Marks = %v, want absence para %s", marks, studentID)
	}

	// El admin ve el llamado en el reporte del estudiante.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/statistics/student/"+studentID, aToken, "")
	if code != http.StatusOK {
		t.Fatalf("student report = %d, want 200: %s", code, raw)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report inválido: %s", raw)
	}
	found := false
	if sessions, ok := report["Sessions"].([]any); ok {
		for _, s := range sessions {
			if m, ok := s.(map[string]any); ok && m["SessionID"] == sessionID && m["Mark"] == "absence" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("report sin la ausencia recién guardada: %s", raw)
	}
}

// El llamado del frontend llega al backend y aparece en admin, y la
// corrección del admin se refleja al releer.
func TestWarningBatchContract(t *testing.T) {
	h := boot(t)
	dToken := flowToken(t, h, "docente", "docente123*")
	aToken := flowToken(t, h, "admin", "admin123*")

	var groupID, studentID string
	for w := 0; w <= 6 && (groupID == "" || studentID == ""); w++ {
		_, raw := flowDo(t, h, http.MethodGet, fmt.Sprintf("/v1/teachers/me/schedule?weekday=%d", w), dToken, "")
		var entries []map[string]any
		if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
			continue
		}
		gid, _ := entries[0]["GroupID"].(string)
		if gid == "" {
			continue
		}
		c, r := flowDo(t, h, http.MethodGet, "/v1/classes/"+gid+"/roster", dToken, "")
		var roster []map[string]any
		if c != http.StatusOK {
			continue
		}
		if err := json.Unmarshal(r, &roster); err != nil || len(roster) == 0 {
			continue
		}
		if sid, ok := roster[0]["StudentID"].(string); ok && sid != "" {
			groupID, studentID = gid, sid
		}
	}
	if groupID == "" {
		t.Fatal("seed sin grupo con nómina para el docente")
	}

	// Cuerpo EXACTO del SPA: sin snapshots (el backend los congela).
	code, raw := flowDo(t, h, http.MethodPost, "/v1/warnings/batch", dToken,
		`{"studentIDs":["`+studentID+`"],"classID":"`+groupID+`","title":"Llamado de prueba","gravity":"moderate","description":"Interrumpe la clase"}`)
	if code != http.StatusCreated {
		t.Fatalf("batch = %d, want 201: %s", code, raw)
	}
	var created []map[string]any
	if err := json.Unmarshal(raw, &created); err != nil || len(created) != 1 || created[0]["ID"] == nil {
		t.Fatalf("batch sin llamado creado: %s", raw)
	}
	warningID := created[0]["ID"].(string)

	// El admin lo ve en el historial de llamados del estudiante.
	readWarnings := func() []byte {
		t.Helper()
		c, r := flowDo(t, h, http.MethodGet, "/v1/students/"+studentID+"/warnings", aToken, "")
		if c != http.StatusOK {
			t.Fatalf("warnings = %d, want 200: %s", c, r)
		}
		return r
	}
	if raw := readWarnings(); !strings.Contains(string(raw), "Llamado de prueba") {
		t.Fatalf("warnings sin el llamado recién creado: %s", raw)
	}

	// Corrección del admin con el cuerpo del SPA y relectura.
	code, raw = flowDo(t, h, http.MethodPut, "/v1/warnings/"+warningID, aToken, `{"description":"Corregido por rectoría"}`)
	if code != http.StatusOK {
		t.Fatalf("correct = %d, want 200: %s", code, raw)
	}
	if raw := readWarnings(); !strings.Contains(string(raw), "Corregido por rectoría") {
		t.Fatalf("warnings tras corrección sin el texto nuevo: %s", raw)
	}

	// Blindaje del contrato estricto: campos desconocidos se rechazan,
	// así el frontend no puede mandar de más en silencio.
	code, raw = flowDo(t, h, http.MethodPost, "/v1/warnings/batch", dToken,
		`{"studentIDs":["`+studentID+`"],"classID":"`+groupID+`","title":"X","gravity":"mild","description":"Y","snapshots":{}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("batch con snapshots = %d, want 400: %s", code, raw)
	}
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/open", dToken, `{"classGroupID":"`+groupID+`","hacker":true}`)
	if code != http.StatusBadRequest {
		t.Fatalf("open con campo extra = %d, want 400: %s", code, raw)
	}
}

// El permiso de docentes se registra con motivo obligatorio implícito
// en la nota y cuenta como excusa, no como falta.
func TestTeacherRollPermission(t *testing.T) {
	h := boot(t)
	aToken := flowToken(t, h, "admin", "admin123*")

	code, raw := flowDo(t, h, http.MethodGet, "/v1/teachers", aToken, "")
	if code != http.StatusOK {
		t.Fatalf("teachers = %d, want 200: %s", code, raw)
	}
	var profiles []map[string]any
	if err := json.Unmarshal(raw, &profiles); err != nil || len(profiles) == 0 {
		t.Fatalf("sin docentes: %s", raw)
	}
	tid, _ := profiles[0]["ID"].(string)

	code, raw = flowDo(t, h, http.MethodPost, "/v1/teachers/attendance", aToken,
		`{"day":"2026-10-30","marks":[{"teacherID":"`+tid+`","status":"permission","note":"Cita médica"}]}`)
	if code != http.StatusOK {
		t.Fatalf("teacher roll permiso = %d, want 200: %s", code, raw)
	}
	code, raw = flowDo(t, h, http.MethodGet, "/v1/teachers/attendance?date=2026-10-30", aToken, "")
	if code != http.StatusOK || !strings.Contains(string(raw), "permission") {
		t.Fatalf("teacher day sin permiso: %d %s", code, raw)
	}
}
