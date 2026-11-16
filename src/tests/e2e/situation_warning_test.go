// Llamados con el catálogo del manual: el docente elige la situación, el
// servidor decide el tipo y el llamado queda en el mismo sistema que leen
// la administración, la ficha del estudiante y "mis llamados".
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSituationWarningFlow(t *testing.T) {
	h := boot(t)
	dToken := flowToken(t, h, "docente", "docente123*")
	aToken := flowToken(t, h, "admin", "admin123*")
	groupID, studentID := mustGroupStudent(t, h, dToken)

	// s2-15 es "Evadirse de clase": tipo 2 aunque el cliente diga otra cosa.
	code, raw := flowDo(t, h, http.MethodPost, "/v1/warnings/batch", dToken,
		`{"classID":"`+groupID+`","situationID":"s2-15","gravity":"mild","title":"Evadirse de clase o de la institución","description":"","studentIDs":["`+studentID+`"]}`)
	if code != http.StatusCreated {
		t.Fatalf("batch = %d, want 201: %s", code, raw)
	}
	var created []map[string]any
	if err := json.Unmarshal(raw, &created); err != nil || len(created) != 1 {
		t.Fatalf("batch body: %s", raw)
	}
	id, _ := created[0]["ID"].(string)
	if g := created[0]["Gravity"]; g != "moderate" {
		t.Errorf("Gravity = %v, want moderate (tipo 2)", g)
	}

	// Situación fuera del catálogo: rechazada.
	code, _ = flowDo(t, h, http.MethodPost, "/v1/warnings/batch", dToken,
		`{"classID":"`+groupID+`","situationID":"s9-99","title":"x","studentIDs":["`+studentID+`"]}`)
	if code < 400 || code >= 500 {
		t.Errorf("situación inválida = %d, want 4xx", code)
	}

	hasID := func(path, token string) bool {
		t.Helper()
		c, r := flowDo(t, h, http.MethodGet, path, token, "")
		if c != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, c, r)
		}
		var list []map[string]any
		_ = json.Unmarshal(r, &list)
		for _, w := range list {
			if w["ID"] == id {
				return true
			}
		}
		return false
	}
	if !hasID("/v1/warnings/mine", dToken) {
		t.Error("el llamado no aparece en /v1/warnings/mine del docente")
	}
	if !hasID("/v1/warnings/recent", aToken) {
		t.Error("el llamado no aparece en la lista del admin")
	}
	if !hasID("/v1/students/"+studentID+"/warnings", aToken) {
		t.Error("el llamado no aparece en la ficha del estudiante")
	}
	// El admin no registró nada: su lista propia no trae el del docente.
	if hasID("/v1/warnings/mine", aToken) {
		t.Error("/v1/warnings/mine del admin trae un llamado ajeno")
	}
}
