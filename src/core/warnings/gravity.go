// Package warnings records llamados de atención: formal notices issued to
// a student by a teacher, graded leve / medio / grave. Each warning freezes
// a snapshot of the student's identity at issue time so the record stays
// readable even after the current profile changes.
package warnings

import "strings"

// Gravity grades how serious a warning is. Stable identifiers; user-facing
// names live in the i18n catalog (warning.gravity.<id>).
type Gravity string

const (
	// GravityMild is a light warning (leve).
	GravityMild Gravity = "mild"
	// GravityModerate is a medium warning (medio).
	GravityModerate Gravity = "moderate"
	// GravitySevere is a grave warning (grave).
	GravitySevere Gravity = "severe"
)

var knownGravities = [...]Gravity{GravityMild, GravityModerate, GravitySevere}

// Parse converts a case-insensitive, whitespace-trimmed string into a
// Gravity, accepting both the stable id and the Spanish term.
func Parse(s string) (Gravity, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if g := Gravity(v); g.IsValid() {
		return g, nil
	}
	switch v {
	case "leve":
		return GravityMild, nil
	case "medio", "moderado":
		return GravityModerate, nil
	case "grave":
		return GravitySevere, nil
	}
	return "", ErrUnknownGravity
}

// IsValid reports whether g is one of the known gravities.
func (g Gravity) IsValid() bool {
	switch g {
	case GravityMild, GravityModerate, GravitySevere:
		return true
	}
	return false
}

// String returns the stable identifier of g.
func (g Gravity) String() string { return string(g) }

// MessageKey returns the i18n catalog key holding the user-facing name.
func (g Gravity) MessageKey() string { return "warning.gravity." + g.String() }

// KnownGravities returns a fresh slice of all gravities in ascending
// order of seriousness.
func KnownGravities() []Gravity {
	out := make([]Gravity, len(knownGravities))
	copy(out, knownGravities[:])
	return out
}
