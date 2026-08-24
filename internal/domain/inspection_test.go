package domain

import (
	"errors"
	"testing"
	"time"
)

func TestInspectionStart(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	inspection := Inspection{
		ID:          "inspection-1",
		Status:      InspectionScheduled,
		ScheduledAt: now.Add(time.Hour),
		Version:     2,
	}
	if err := inspection.Start(now); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if inspection.Status != InspectionExecuting {
		t.Fatalf("status = %s, want executing", inspection.Status)
	}
	if inspection.Version != 3 {
		t.Fatalf("version = %d, want 3", inspection.Version)
	}
	if inspection.StartedAt.Location() != time.UTC {
		t.Fatalf("started location = %s, want UTC", inspection.StartedAt.Location())
	}
	if !inspection.StartedAt.Equal(now) {
		t.Fatalf("started_at = %s, want %s", inspection.StartedAt, now)
	}
}

func TestInspectionStartRejectsEveryNonScheduledState(t *testing.T) {
	states := []InspectionStatus{
		InspectionExecuting,
		InspectionPassed,
		InspectionFailed,
		InspectionStatus(""),
		InspectionStatus("canceled"),
	}
	for _, status := range states {
		t.Run(string(status), func(t *testing.T) {
			inspection := Inspection{Status: status, Version: 7}
			err := inspection.Start(time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Start() error = %v, want conflict", err)
			}
			if inspection.Status != status || inspection.Version != 7 || !inspection.StartedAt.IsZero() {
				t.Fatalf("failed start mutated inspection: %+v", inspection)
			}
		})
	}
}

func TestInspectionCompletePassedAndFailed(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		passed     bool
		unresolved int
		want       InspectionStatus
	}{
		{name: "passes when no findings remain", passed: true, unresolved: 0, want: InspectionPassed},
		{name: "fails with no recorded finding", passed: false, unresolved: 0, want: InspectionFailed},
		{name: "fails with one finding", passed: false, unresolved: 1, want: InspectionFailed},
		{name: "fails with several findings", passed: false, unresolved: 8, want: InspectionFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inspection := Inspection{Status: InspectionExecuting, Version: 5}
			if err := inspection.Complete(test.passed, test.unresolved, now); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if inspection.Status != test.want {
				t.Fatalf("status = %s, want %s", inspection.Status, test.want)
			}
			if inspection.Version != 6 {
				t.Fatalf("version = %d, want 6", inspection.Version)
			}
			if !inspection.CompletedAt.Equal(now) {
				t.Fatalf("completed_at = %s, want %s", inspection.CompletedAt, now)
			}
		})
	}
}

func TestInspectionCannotPassWithOpenFindings(t *testing.T) {
	for _, unresolved := range []int{1, 2, 10, 100} {
		inspection := Inspection{Status: InspectionExecuting, Version: 9}
		err := inspection.Complete(true, unresolved, time.Now())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Complete(true, %d) error = %v, want conflict", unresolved, err)
		}
		if inspection.Status != InspectionExecuting || inspection.Version != 9 || !inspection.CompletedAt.IsZero() {
			t.Fatalf("failed completion mutated inspection: %+v", inspection)
		}
	}
}

func TestInspectionCannotCompleteBeforeOrAfterExecution(t *testing.T) {
	states := []InspectionStatus{
		InspectionScheduled,
		InspectionPassed,
		InspectionFailed,
		InspectionStatus("canceled"),
	}
	for _, status := range states {
		for _, passed := range []bool{false, true} {
			inspection := Inspection{Status: status, Version: 3}
			err := inspection.Complete(passed, 0, time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("status %s passed %v error = %v, want conflict", status, passed, err)
			}
			if inspection.Status != status || inspection.Version != 3 {
				t.Fatalf("failed completion mutated inspection: %+v", inspection)
			}
		}
	}
}

func TestFindingResolve(t *testing.T) {
	now := time.Date(2026, 9, 3, 7, 0, 0, 0, time.UTC)
	finding := Finding{
		ID:       "finding-1",
		Severity: SeverityMajor,
		Summary:  "coating dry-film thickness below approved range",
		DueAt:    now.Add(24 * time.Hour),
		Version:  1,
	}
	if err := finding.Resolve("surface prepared and coating reapplied", now); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if finding.Resolution != "surface prepared and coating reapplied" {
		t.Fatalf("resolution = %q", finding.Resolution)
	}
	if !finding.ResolvedAt.Equal(now) {
		t.Fatalf("resolved_at = %s, want %s", finding.ResolvedAt, now)
	}
	if finding.Version != 2 {
		t.Fatalf("version = %d, want 2", finding.Version)
	}
}

func TestFindingResolveRejectsMissingResolutionAndRepeatedClosure(t *testing.T) {
	finding := Finding{ID: "finding-1", Version: 1}
	err := finding.Resolve("", time.Now())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty resolution error = %v, want invalid", err)
	}
	if !finding.ResolvedAt.IsZero() || finding.Version != 1 {
		t.Fatalf("empty resolution mutated finding: %+v", finding)
	}
	now := time.Now().UTC()
	if err := finding.Resolve("repaired", now); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	err = finding.Resolve("duplicate closure", now.Add(time.Hour))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second Resolve() error = %v, want conflict", err)
	}
	if finding.Resolution != "repaired" || !finding.ResolvedAt.Equal(now) || finding.Version != 2 {
		t.Fatalf("second resolution mutated finding: %+v", finding)
	}
}
