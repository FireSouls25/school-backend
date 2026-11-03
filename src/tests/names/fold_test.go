package names_test

import (
	"testing"

	"grade/src/core/names"
)

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"  Matemáticas  ": "matematicas",
		"MATEMATICAS":     "matematicas",
		"Matemáticas":     "matematicas",
		"Juan  Pérez":     "juan perez",
		"  JUAN PÉREZ ":   "juan perez",
		"Muñoz":           "muñoz",
		"MUÑOZ":           "muñoz",
		"Pingüino GÜIRA":  "pinguino guira",
		"":                "",
		"  ":              "",
	} {
		if got := names.Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	// Ñ is a distinct letter: never folded into n.
	if names.Fold("Muñoz") == names.Fold("Munoz") {
		t.Error("Fold folded ñ into n; they must stay distinct")
	}
}
