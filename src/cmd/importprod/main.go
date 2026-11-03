// Command importprod bulk-loads the real 2026 timetable through the real
// core services (TEMPORARY, not wired into the build).
// Usage:
//
//	go run ./src/cmd/importprod --db "$DSN" --dir /tmp/opencode/importdata
//
// CSVs: teachers.csv names,surnames,document,phone,email,homeroom
//
//	subjects.csv name,code,grades,color (grades "6;7;8")
//	schedule.csv group,subject,teacher,weekday,start,end
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"grade/src/core/classes"
	"grade/src/core/schedules"
	"grade/src/core/schoolyears"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	pg "grade/src/platform/postgres"
)

type teachesChecker struct{ subjects *subjects.Service }

func (c teachesChecker) VariantsOf(ctx context.Context, subjectID string) ([]string, error) {
	return nil, nil
}

func (c teachesChecker) Teaches(ctx context.Context, teacherID, subjectID string) (bool, error) {
	current, err := c.subjects.CurrentForTeacher(ctx, teacherID)
	if err != nil {
		return false, err
	}
	for _, a := range current {
		if a.SubjectID == subjectID {
			return true, nil
		}
	}
	return false, nil
}

func readCSV(dir, name string, ncols int) [][]string {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		log.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("parse %s: %v", name, err)
	}
	if len(rows) < 1 {
		log.Fatalf("%s: missing header", name)
	}
	out := rows[1:]
	for i, r := range out {
		for len(r) < ncols {
			r = append(r, "")
		}
		out[i] = r
	}
	return out
}

func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

func normName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func main() {
	dbURL := flag.String("db", os.Getenv("DATABASE_URL"), "postgres DSN")
	dir := flag.String("dir", ".", "CSV directory")
	flag.Parse()
	if *dbURL == "" {
		log.Fatal("--db required")
	}
	ctx := context.Background()
	db, err := pg.Connect(ctx, *dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()

	yearsSvc := schoolyears.NewService(pg.NewSchoolYearsStore(db))
	classesSvc := classes.NewService(pg.NewClassesStore(db))
	subjectsSvc := subjects.NewService(pg.NewSubjectsStore(db))
	teachersSvc := teachers.NewService(pg.NewTeachersStore(db))
	schedulesSvc := schedules.NewService(pg.NewSchedulesStore(db), teachesChecker{subjectsSvc})

	// Verification year (placeholder until the school sets real dates:
	// Jan 1 to Dec 31 with the automatic Colombian holidays).
	year, err := yearsSvc.Create(ctx, schoolyears.SchoolYear{
		ID: uuid.NewString(), Year: 2026, Periods: 3,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		Holidays:  schoolyears.ColombianHolidays(2026),
	})
	if err != nil {
		log.Fatalf("year: %v", err)
	}
	fmt.Println("year:", year.ID)

	// Subjects.
	subjectIDs := map[string]string{}
	for i, r := range readCSV(*dir, "subjects.csv", 4) {
		name := cell(r, 0)
		if name == "" {
			log.Fatalf("subjects.csv row %d: empty name", i+2)
		}
		if _, ok := subjectIDs[normName(name)]; ok {
			log.Fatalf("subjects.csv row %d: duplicate %q", i+2, name)
		}
		var grades []int
		for _, g := range strings.Split(cell(r, 2), ";") {
			if g = strings.TrimSpace(g); g == "" {
				continue
			}
			n, err := strconv.Atoi(g)
			if err != nil {
				log.Fatalf("subjects.csv row %d: bad grade %q", i+2, g)
			}
			grades = append(grades, n)
		}
		created, err := subjectsSvc.CreateSubject(ctx, subjects.Subject{
			Name: name, Code: cell(r, 1), Grades: grades, Color: cell(r, 3), Active: true,
		})
		if err != nil {
			log.Fatalf("subjects.csv row %d (%s): %v", i+2, name, err)
		}
		subjectIDs[normName(name)] = created.ID
	}

	// Groups + teachers first pass (collect labels).
	schedRows := readCSV(*dir, "schedule.csv", 7)
	teacherRows := readCSV(*dir, "teachers.csv", 6)
	wantGroups := map[string]bool{}
	for _, r := range schedRows {
		wantGroups[cell(r, 0)] = true
	}
	for _, r := range teacherRows {
		for _, h := range strings.Split(cell(r, 5), ";") {
			if h = strings.TrimSpace(h); h != "" {
				wantGroups[h] = true
			}
		}
	}
	groupIDs := map[string]string{}
	for label := range wantGroups {
		parts := strings.Split(label, "-")
		if len(parts) != 2 {
			log.Fatalf("bad group %q", label)
		}
		grade, err1 := strconv.Atoi(parts[0])
		group, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			log.Fatalf("bad group %q", label)
		}
		created, err := classesSvc.Create(ctx, classes.ClassGroup{
			SchoolYearID: year.ID, Grade: grade, GroupNo: group,
		})
		if err != nil {
			log.Fatalf("group %s: %v", label, err)
		}
		groupIDs[label] = created.ID
	}

	// Teachers by normalized full name.
	teacherIDs := map[string]string{}
	for i, r := range teacherRows {
		names, surnames := cell(r, 0), cell(r, 1)
		if names == "" || surnames == "" {
			log.Fatalf("teachers.csv row %d: missing names", i+2)
		}
		// Both orders, since display order varies (PDF surnames-first
		// vs corrected names-first splits).
		key := normName(surnames + " " + names)
		if _, ok := teacherIDs[key]; ok {
			log.Fatalf("teachers.csv row %d: duplicate %q", i+2, key)
		}
		created, err := teachersSvc.Create(ctx, teachers.Teacher{
			Names: names, Surnames: surnames, DocumentID: cell(r, 2),
			Phone: cell(r, 3), Email: cell(r, 4), HomeroomClassID: cell(r, 5),
			Active: true,
		})
		if err != nil {
			log.Fatalf("teachers.csv row %d (%s): %v", i+2, key, err)
		}
		teacherIDs[key] = created.ID
		teacherIDs[normName(names+" "+surnames)] = created.ID
	}

	// Assignments + entries. Verification mode: collect every failure
	// and report at the end instead of aborting at the first.
	nAssign, nSlots, nFail := 0, 0, 0
	var failures []string
	fail := func(format string, args ...any) {
		nFail++
		failures = append(failures, fmt.Sprintf(format, args...))
	}
	for i, r := range schedRows {
		row := i + 2
		label, subjName, teachName := cell(r, 0), cell(r, 1), cell(r, 2)
		weekday, err := strconv.Atoi(cell(r, 3))
		if err != nil {
			fail("row %d: bad weekday", row)
			continue
		}
		gid, ok := groupIDs[label]
		if !ok {
			fail("row %d: unknown group %q", row, label)
			continue
		}
		sid, ok := subjectIDs[normName(subjName)]
		if !ok {
			fail("row %d: unknown subject %q", row, subjName)
			continue
		}
		tid, ok := teacherIDs[normName(teachName)]
		if !ok {
			fail("row %d: unknown teacher %q", row, teachName)
			continue
		}
		teaches, err := teachesChecker{subjectsSvc}.Teaches(ctx, tid, sid)
		if err != nil {
			fail("row %d: assignment check: %v", row, err)
			continue
		}
		if !teaches {
			if _, err := subjectsSvc.Assign(ctx, tid, sid, year.StartDate); err != nil {
				fail("row %d: assign: %v", row, err)
				continue
			}
			nAssign++
		}
		if _, err := schedulesSvc.Create(ctx, schedules.Entry{
			ClassGroupID: gid, TeacherID: tid, SubjectID: sid, Weekday: weekday,
			Start: schedules.MustClock(cell(r, 4)), End: schedules.MustClock(cell(r, 5)),
		}); err != nil {
			fail("row %d: %v", row, err)
			continue
		}
		nSlots++
	}
	fmt.Printf("import done: %d subjects, %d groups, %d teachers, %d assignments, %d slots, %d failures\n",
		len(subjectIDs), len(groupIDs), len(teacherIDs), nAssign, nSlots, nFail)
	for _, f := range failures {
		fmt.Println("FAIL:", f)
	}
	if nFail > 0 {
		os.Exit(1)
	}
}
