// Package permits records attendance permissions granted before a class
// ("permisos previos"): one student, one class-group, one school day,
// with the reason and the author. A valid permit locks the roll call
// for that student: the teacher still sees the student in the list but
// cannot mark present, absent, late or evasion, because the permission
// already covers the class.
package permits

import "time"

// Permit is one previously granted absence permission. Date is the day
// the permission covers, normalized to UTC midnight so comparisons are
// calendar-day comparisons.
type Permit struct {
	// ID is the unique, immutable UUID of the permit.
	ID string
	// StudentID references the covered student.
	StudentID string
	// ClassGroupID references the class-group the permit applies to.
	ClassGroupID string
	// SubjectID references one subject when the permit only covers that
	// class; empty means the whole school day for the group.
	SubjectID string
	// Date is the covered day.
	Date time.Time
	// Reason is the motive written by the granting office.
	Reason string
	// CreatedBy identifies who granted the permit (opaque subject id).
	CreatedBy string
	// CreatedAt is when the permit was recorded, set by the Service.
	CreatedAt time.Time
}

// Draft is the granting office's input. Date is an ISO day
// ("2006-01-02"); empty means today.
type Draft struct {
	StudentID    string
	ClassGroupID string
	SubjectID    string
	Date         string
	Reason       string
	CreatedBy    string
}

// AppliesTo reports whether the permit covers the given class-group and
// subject. An empty subject on the permit covers every class of the day.
func (p Permit) AppliesTo(classGroupID, subjectID string) bool {
	if p.ClassGroupID != classGroupID {
		return false
	}
	return p.SubjectID == "" || subjectID == "" || p.SubjectID == subjectID
}
