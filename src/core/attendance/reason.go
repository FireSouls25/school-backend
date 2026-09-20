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
)

var knownReasons = [...]Reason{ReasonAbsence, ReasonEvasion}

// Parse converts a case-insensitive, whitespace-trimmed string into a Reason.
func Parse(s string) (Reason, error) {
	r := Reason(strings.ToLower(strings.TrimSpace(s)))
	if !r.IsValid() {
		return "", ErrUnknownReason
	}
	return r, nil
}

// IsValid reports whether r is one of the known reasons.
func (r Reason) IsValid() bool {
	switch r {
	case ReasonAbsence, ReasonEvasion:
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
