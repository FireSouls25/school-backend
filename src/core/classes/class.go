// Package classes owns the class-groups (salones): one grade+group within
// one school year, e.g. 7-1/2026. Groups are per-year rows, so adding or
// removing a group (6-5 exists this year but not the next) means creating
// it or not. Enrollments reference a group by its id.
package classes

import "fmt"

// MinGrade and MaxGrade bound Colombian school grades.
const MinGrade = 1

const MaxGrade = 11

// MinGroupNo bounds group numbers within a grade.
const MinGroupNo = 1

// ClassGroup is one classroom: grade+group within a school year.
type ClassGroup struct {
	// ID is the unique, immutable UUID of the class-group.
	ID string
	// SchoolYearID references the school year. Opaque here.
	SchoolYearID string
	// Grade is the school grade, 1 to 11.
	Grade int
	// GroupNo is the group number within the grade (9-1 → 1).
	GroupNo int
}

// Label returns the display label of the group (e.g. "7-1").
func (g ClassGroup) Label() string {
	return fmt.Sprintf("%d-%d", g.Grade, g.GroupNo)
}
