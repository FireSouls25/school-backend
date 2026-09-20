// Package incidents records faults and misbehavior for students, graded by
// severity: leve, normal and grave.
package incidents

import "strings"

// Severity grades how serious a fault is. Stable identifiers; user-facing
// names live in the i18n catalog (severity.<id>).
type Severity string

const (
	// SeverityMinor is a light fault (leve).
	SeverityMinor Severity = "minor"
	// SeverityOrdinary is a regular fault (normal).
	SeverityOrdinary Severity = "ordinary"
	// SeveritySevere is a grave fault (grave).
	SeveritySevere Severity = "severe"
)

var knownSeverities = [...]Severity{SeverityMinor, SeverityOrdinary, SeveritySevere}

// Parse converts a case-insensitive, whitespace-trimmed string into a
// Severity, accepting both the stable id and the Spanish term.
func Parse(s string) (Severity, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	sv := Severity(v)
	if sv.IsValid() {
		return sv, nil
	}
	switch v {
	case "leve":
		return SeverityMinor, nil
	case "normal":
		return SeverityOrdinary, nil
	case "grave":
		return SeveritySevere, nil
	}
	return "", ErrUnknownSeverity
}

// IsValid reports whether s is one of the known severities.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityMinor, SeverityOrdinary, SeveritySevere:
		return true
	}
	return false
}

// String returns the stable identifier of s.
func (s Severity) String() string { return string(s) }

// MessageKey returns the i18n catalog key holding the user-facing name.
func (s Severity) MessageKey() string { return "severity." + s.String() }

// KnownSeverities returns a fresh slice of all severities in ascending
// order of seriousness.
func KnownSeverities() []Severity {
	out := make([]Severity, len(knownSeverities))
	copy(out, knownSeverities[:])
	return out
}
