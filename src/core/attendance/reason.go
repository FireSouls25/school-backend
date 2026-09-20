// Package attendance records and queries class assistance history for
// students, across their current and previous class-groups.
package attendance

import "strings"

// Reason classifies one attendance entry. Stable identifiers; user-facing
// names live in the i18n catalog (reason.<id>).
type Reason string

const (
	// ReasonAbsence is a plain absence (inasistencia).
	ReasonAbsence Reason = "absence"
	// ReasonEvasion is leaving or skipping class (evasión).
	ReasonEvasion Reason = "evasion"
	// ReasonLate is arriving late to class (atraso).
	ReasonLate Reason = "late"
)

var knownReasons = [...]Reason{ReasonAbsence, ReasonEvasion, ReasonLate}

// Parse converts a case-insensitive, whitespace-trimmed string into a
// Reason, accepting both the stable id and the Spanish term.
func Parse(s string) (Reason, error) {
	v := stripAccents(strings.ToLower(strings.TrimSpace(s)))
	if r := Reason(v); r.IsValid() {
		return r, nil
	}
	switch v {
	case "inasistencia":
		return ReasonAbsence, nil
	case "evasion":
		return ReasonEvasion, nil
	case "atraso":
		return ReasonLate, nil
	}
	return "", ErrUnknownReason
}

// stripAccents folds Spanish accented vowels so "evasión" matches "evasion".
func stripAccents(s string) string {
	r := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	)
	return r.Replace(s)
}

// IsValid reports whether r is one of the known reasons.
func (r Reason) IsValid() bool {
	switch r {
	case ReasonAbsence, ReasonEvasion, ReasonLate:
		return true
	}
	return false
}

// String returns the stable identifier of r.
func (r Reason) String() string { return string(r) }

// MessageKey returns the i18n catalog key holding the user-facing name.
func (r Reason) MessageKey() string { return "reason." + r.String() }

// KnownReasons returns a fresh slice of all reasons in stable order.
func KnownReasons() []Reason {
	out := make([]Reason, len(knownReasons))
	copy(out, knownReasons[:])
	return out
}
