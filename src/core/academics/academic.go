// Package academics owns academic alerts: a teacher flags one or more
// students whose behavior or performance needs admin review. Alerts are
// not grades (the platform does not support them) and not warnings:
// they carry no gravity, only a reason, and they stay open until an
// admin reviews them. Admins treat open alerts like repeated serious
// warnings: bell, pendientes and a mark on the student file.
package academics

import (
	"context"
	"time"
)

// Status is the review state of an alert.
type Status string

const (
	// StatusOpen needs admin review.
	StatusOpen Status = "open"
	// StatusReviewed was already checked by an admin.
	StatusReviewed Status = "reviewed"
)

// Alert is one academic alert against one student.
type Alert struct {
	ID         string
	StudentID  string
	TeacherID  string
	Reason     string
	Status     Status
	CreatedAt  time.Time
	ReviewedAt time.Time
	ReviewedBy string
}

// Input carries the fields needed to raise alerts. IDs are assigned by
// the Service.
type Input struct {
	StudentID string
	TeacherID string
	Reason    string
}

// Store persists alerts.
type Store interface {
	Add(ctx context.Context, a Alert) (Alert, error)
	ByID(ctx context.Context, id string) (Alert, error)
	// Open returns unreviewed alerts, newest first, optionally filtered
	// by student (empty means all).
	Open(ctx context.Context, studentID string) ([]Alert, error)
	// ForStudent returns every alert of one student, newest first.
	ForStudent(ctx context.Context, studentID string) ([]Alert, error)
	Update(ctx context.Context, a Alert) (Alert, error)
}
