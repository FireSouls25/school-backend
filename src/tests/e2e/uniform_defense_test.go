// Uniforme y descargos: el docente registra faltas de uniforme (checkbox
// por estudiante, activo por defecto) y la secretaría/admin registra la
// defensa escrita ante una falta. Ambos viajan a reportes e historial.
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func mustGroupStudent(t *testing.T, h http.Handler, token string) (string, string) {
	t.Helper()
	for w := 0; w <= 6; w++ {
		code, raw := flowDo(t, h, http.MethodGet, fmt.Sprintf("/v1/teachers/me/schedule?weekday=%d", w), token, "")
		if code != http.StatusOK {
			t.Fatalf("schedule w=%d = %d, want 200: %s", w, code, raw)
		}
		var entries []map[string]any
		if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
			continue
		}
		gid, _ := entries[0]["GroupID"].(string)
		code, raw = flowDo(t, h, http.MethodGet, "/v1/classes/"+gid+"/roster", token, "")
		if code != http.StatusOK {
			continue
		}
		var roster []map[string]any
		if err := json.Unmarshal(raw, &roster); err != nil || len(roster) == 0 {
			continue
		}
		if sid, ok := roster[0]["StudentID"].(string); ok && sid != "" {
			return gid, sid
		}
	}
	t.Fatal("seed sin grupo con nómina para el docente")
	return "", ""
}

func TestUniformAndDefenseFlow(t *testing.T) {
	h := boot(t)
	dToken := flowToken(t, h, "docente", "docente123*")
	aToken := flowToken(t, h, "admin", "admin123*")
	sToken := flowToken(t, h, "secretaria", "secretaria123*")
	groupID, studentID := mustGroupStudent(t, h, dToken)

	// Abrir sesión y marcar falta: base para uniforme y descargo.
	code, raw := flowDo(t, h, http.MethodPost, "/v1/sessions/open", dToken, `{"classGroupID":"`+groupID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open = %d, want 201: %s", code, raw)
	}
	var opened map[string]any
	_ = json.Unmarshal(raw, &opened)
	sessionID, _ := opened["ID"].(string)
	mark := func(sid, m string) {
		t.Helper()
		c, r := flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/marks", dToken,
			`{"studentID":"`+sid+`","mark":"`+m+`","note":""}`)
		if c != http.StatusCreated {
			t.Fatalf("mark %s=%q: %d, want 201: %s", sid, m, c, r)
		}
	}
	mark(studentID, "absence")

	// Uniforme: el docente registra la falta.
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/uniform", dToken,
		`{"studentID":"`+studentID+`","ok":false}`)
	if code != http.StatusCreated {
		t.Fatalf("uniform = %d, want 201: %s", code, raw)
	}
	// Repetir el mismo estado es no-op aceptado.
	code, _ = flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/uniform", dToken,
		`{"studentID":"`+studentID+`","ok":false}`)
	if code != http.StatusOK && code != http.StatusBadRequest {
		t.Fatalf("uniform repetido = %d, want 200/400: ", code)
	}
	// El detalle trae uniforme e historial.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/sessions/"+sessionID, dToken, "")
	if code != http.StatusOK {
		t.Fatalf("detail = %d, want 200: %s", code, raw)
	}
	var detail map[string]any
	_ = json.Unmarshal(raw, &detail)
	if uniform, ok := detail["Uniform"].(map[string]any); !ok || uniform[studentID] != false {
		t.Fatalf("detail sin falta de uniforme: %s", raw)
	}
	if hist, ok := detail["UniformHistory"].([]any); !ok || len(hist) == 0 {
		t.Fatalf("detail sin historial de uniforme: %s", raw)
	}
	// El reporte cuenta la falta de uniforme.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/statistics/student/"+studentID, aToken, "")
	if code != http.StatusOK {
		t.Fatalf("report = %d, want 200: %s", code, raw)
	}
	var report map[string]any
	_ = json.Unmarshal(raw, &report)
	if n, _ := report["UniformFaults"].(float64); n < 1 {
		t.Fatalf("report sin UniformFaults: %s", raw)
	}
	found := false
	if sessions, ok := report["Sessions"].([]any); ok {
		for _, s := range sessions {
			if m, ok := s.(map[string]any); ok && m["SessionID"] == sessionID && m["UniformOK"] == false {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("report sin UniformOK=false en la sesión: %s", raw)
	}

	// Descargos: el docente no puede, la secretaría y el admin sí.
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/defenses", dToken,
		`{"studentID":"`+studentID+`","text":"Llegué tarde por el bus"}`)
	if code != http.StatusForbidden {
		t.Fatalf("defense docente = %d, want 403: %s", code, raw)
	}
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/defenses", sToken,
		`{"studentID":"`+studentID+`","text":"Se varó el bus de la vereda"}`)
	if code != http.StatusCreated {
		t.Fatalf("defense secretaría = %d, want 201: %s", code, raw)
	}
	// El detalle la muestra junto a la falta.
	code, raw = flowDo(t, h, http.MethodGet, "/v1/sessions/"+sessionID, dToken, "")
	var detail2 map[string]any
	_ = json.Unmarshal(raw, &detail2)
	if defs, ok := detail2["Defenses"].(map[string]any); !ok {
		t.Fatalf("detail sin Defenses: %s", raw)
	} else if d, ok := defs[studentID].(map[string]any); !ok || d["Text"] != "Se varó el bus de la vereda" {
		t.Fatalf("defense ausente o distinta: %s", raw)
	}
	// Sin falta no hay descargo: presente no se defiende.
	mark(studentID, "")
	code, raw = flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/defenses", sToken,
		`{"studentID":"`+studentID+`","text":"Ya no aplica"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("defense en presente = %d, want 400: %s", code, raw)
	}
}
