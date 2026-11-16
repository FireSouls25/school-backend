// Un permiso que la secretaría registra sobre una falta ya tomada manda
// sobre el docente: la lista lo muestra bloqueado ("Con permiso") y el
// servidor rechaza que el docente lo cambie.
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestOfficePermissionLocksTeacher(t *testing.T) {
	h := boot(t)
	dToken := flowToken(t, h, "docente", "docente123*")
	sToken := flowToken(t, h, "secretaria", "secretaria123*")
	groupID, studentID := mustGroupStudent(t, h, dToken)

	code, raw := flowDo(t, h, http.MethodPost, "/v1/sessions/open", dToken, `{"classGroupID":"`+groupID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open = %d: %s", code, raw)
	}
	var opened map[string]any
	_ = json.Unmarshal(raw, &opened)
	sessionID, _ := opened["ID"].(string)
	mark := func(token, m, note string) int {
		c, _ := flowDo(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/marks", token,
			`{"studentID":"`+studentID+`","mark":"`+m+`","note":"`+note+`"}`)
		return c
	}
	if c := mark(dToken, "absence", ""); c != http.StatusCreated {
		t.Fatalf("docente falta = %d", c)
	}
	// El docente puede dar y quitar su propio permiso.
	if c := mark(dToken, "absence-permission", "Enfermería"); c != http.StatusCreated {
		t.Fatalf("docente permiso = %d", c)
	}
	if c := mark(dToken, "absence", ""); c != http.StatusCreated {
		t.Fatalf("docente quita su permiso = %d", c)
	}
	// La secretaría registra el permiso: queda bloqueado para el docente.
	if c := mark(sToken, "absence-permission", "Cita médica"); c != http.StatusCreated {
		t.Fatalf("secretaría permiso = %d", c)
	}
	if c := mark(dToken, "", ""); c != http.StatusConflict {
		t.Errorf("docente cambia permiso de secretaría = %d, want 409", c)
	}
	_, raw = flowDo(t, h, http.MethodGet, "/v1/sessions/"+sessionID, dToken, "")
	var detail struct {
		OfficeLocks map[string]string
	}
	_ = json.Unmarshal(raw, &detail)
	if detail.OfficeLocks[studentID] != "Cita médica" {
		t.Errorf("OfficeLocks = %v, want the student with its reason", detail.OfficeLocks)
	}
	// La secretaría sí puede corregirlo.
	if c := mark(sToken, "absence", ""); c != http.StatusCreated {
		t.Errorf("secretaría corrige = %d", c)
	}
}
