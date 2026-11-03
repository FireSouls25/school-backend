package observations

import (
	"fmt"
	"time"
)

// Kind ranks a situation: 1 (Tipo I, leve), 2 (Tipo II, medio) or
// 3 (Tipo III, grave). The server always decides it from the catalog;
// teachers only pick the situation.
type Kind int

const (
	KindOne   Kind = 1
	KindTwo   Kind = 2
	KindThree Kind = 3
)

// Situation is one catalog entry: a stable id plus its server-decided
// kind. Display texts live in the frontend i18n (indexed by position
// inside each kind block), never here.
type Situation struct {
	ID   string
	Kind Kind
}

// kindSizes mirrors the manual: 14 Tipo I, 43 Tipo II, 8 Tipo III.
var kindSizes = []struct {
	kind  Kind
	count int
}{
	{KindOne, 14},
	{KindTwo, 43},
	{KindThree, 8},
}

// Catalog returns the 65 fixed situations with stable ids ("s1-01"
// through "s3-08"). The list never changes at runtime; only the
// manual (and the frontend texts) can extend it.
func Catalog() []Situation {
	out := make([]Situation, 0, 65)
	for _, k := range kindSizes {
		for n := 1; n <= k.count; n++ {
			out = append(out, Situation{ID: fmt.Sprintf("s%d-%02d", int(k.kind), n), Kind: k.kind})
		}
	}
	return out
}

// KindOf resolves the server-decided kind of a situation id, reporting
// whether the id belongs to the catalog.
func KindOf(id string) (Kind, bool) {
	for _, s := range Catalog() {
		if s.ID == id {
			return s.Kind, true
		}
	}
	return 0, false
}

// Observation is one recorded convivencia situation: the teacher, the
// student, the server-decided kind and an optional free note. Names
// are frozen snapshots: roster edits never rewrite history.
type Observation struct {
	ID           string
	StudentID    string
	StudentName  string
	TeacherID    string
	ClassGroupID string
	ClassLabel   string
	SituationID  string
	Kind         Kind
	Note         string
	// Date is the day of the facts (not the record time), normalized
	// to UTC midnight.
	Date time.Time
}

// Draft is the teacher's input: the situation (kind derived), an
// optional note and an optional ISO day (default today).
type Draft struct {
	StudentID    string
	ClassGroupID string
	SituationID  string
	Note         string
	Date         string
}
