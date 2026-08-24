package domain

import (
	"errors"
	"testing"
	"time"
)

func validWorkPackage() WorkPackage {
	return WorkPackage{
		ID:      "work-1",
		Code:    "TOWER-COAT-01",
		Title:   "North tower protective coating",
		Scope:   "Complete coating inspection and repair",
		Risk:    RiskHigh,
		Status:  WorkPlanned,
		OwnerID: "contractor-1",
		DueAt:   time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
		Version: 1,
	}
}

func TestWorkPackageValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WorkPackage)
		field  string
	}{
		{name: "valid", mutate: func(*WorkPackage) {}},
		{name: "code required", mutate: func(w *WorkPackage) { w.Code = "" }, field: "code"},
		{name: "title required", mutate: func(w *WorkPackage) { w.Title = "" }, field: "title"},
		{name: "scope required", mutate: func(w *WorkPackage) { w.Scope = "" }, field: "scope"},
		{name: "owner required", mutate: func(w *WorkPackage) { w.OwnerID = "" }, field: "owner_id"},
		{name: "risk required", mutate: func(w *WorkPackage) { w.Risk = "" }, field: "risk"},
		{name: "unknown risk", mutate: func(w *WorkPackage) { w.Risk = "extreme" }, field: "risk"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := validWorkPackage()
			test.mutate(&work)
			err := work.Validate()
			if test.field == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.field != "" {
				var fieldError *FieldError
				if !errors.As(err, &fieldError) {
					t.Fatalf("Validate() error = %v, want FieldError", err)
				}
				if fieldError.Field != test.field {
					t.Fatalf("field = %q, want %q", fieldError.Field, test.field)
				}
			}
		})
	}
}

func TestWorkPackageCompleteLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	work := validWorkPackage()
	steps := []struct {
		from WorkStatus
		to   WorkStatus
	}{
		{from: WorkPlanned, to: WorkActive},
		{from: WorkActive, to: WorkSubmitted},
		{from: WorkSubmitted, to: WorkRework},
		{from: WorkRework, to: WorkActive},
		{from: WorkActive, to: WorkSubmitted},
		{from: WorkSubmitted, to: WorkAccepted},
	}
	for index, step := range steps {
		if work.Status != step.from {
			t.Fatalf("step %d begins at %s, want %s", index, work.Status, step.from)
		}
		previousVersion := work.Version
		if err := work.Transition(step.to, now.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatalf("step %d transition error = %v", index, err)
		}
		if work.Status != step.to {
			t.Fatalf("step %d status = %s, want %s", index, work.Status, step.to)
		}
		if work.Version != previousVersion+1 {
			t.Fatalf("step %d version = %d, want %d", index, work.Version, previousVersion+1)
		}
	}
}

func TestWorkPackageRejectsIllegalTransitionsWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		from WorkStatus
		to   WorkStatus
	}{
		{name: "planned cannot submit", from: WorkPlanned, to: WorkSubmitted},
		{name: "planned cannot accept", from: WorkPlanned, to: WorkAccepted},
		{name: "active cannot accept", from: WorkActive, to: WorkAccepted},
		{name: "submitted cannot become active", from: WorkSubmitted, to: WorkActive},
		{name: "rework cannot submit directly", from: WorkRework, to: WorkSubmitted},
		{name: "accepted cannot reopen", from: WorkAccepted, to: WorkActive},
		{name: "accepted cannot repeat", from: WorkAccepted, to: WorkAccepted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := validWorkPackage()
			work.Status = test.from
			work.Version = 10
			err := work.Transition(test.to, time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Transition() error = %v, want conflict", err)
			}
			if work.Status != test.from || work.Version != 10 || !work.UpdatedAt.IsZero() {
				t.Fatalf("failed transition mutated work: %+v", work)
			}
		})
	}
}

func TestWorkPackageOpeningBlockers(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		status WorkStatus
		risk   RiskLevel
		dueAt  time.Time
		blocks bool
	}{
		{name: "accepted never blocks", status: WorkAccepted, risk: RiskCritical, dueAt: now.Add(-time.Hour), blocks: false},
		{name: "critical active blocks", status: WorkActive, risk: RiskCritical, dueAt: now.Add(time.Hour), blocks: true},
		{name: "high submitted blocks", status: WorkSubmitted, risk: RiskHigh, dueAt: now.Add(time.Hour), blocks: true},
		{name: "medium overdue blocks", status: WorkActive, risk: RiskMedium, dueAt: now.Add(-time.Minute), blocks: true},
		{name: "low overdue blocks", status: WorkRework, risk: RiskLow, dueAt: now.Add(-time.Minute), blocks: true},
		{name: "medium future does not block", status: WorkActive, risk: RiskMedium, dueAt: now.Add(time.Hour), blocks: false},
		{name: "low future does not block", status: WorkPlanned, risk: RiskLow, dueAt: now.Add(time.Hour), blocks: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			work := validWorkPackage()
			work.Status = test.status
			work.Risk = test.risk
			work.DueAt = test.dueAt
			if got := work.BlocksOpening(now); got != test.blocks {
				t.Fatalf("BlocksOpening() = %v, want %v", got, test.blocks)
			}
		})
	}
}
