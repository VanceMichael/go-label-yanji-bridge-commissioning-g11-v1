package domain

import "time"

type InspectionStatus string
type FindingSeverity string

const (
	InspectionScheduled InspectionStatus = "scheduled"
	InspectionExecuting InspectionStatus = "executing"
	InspectionPassed    InspectionStatus = "passed"
	InspectionFailed    InspectionStatus = "failed"

	SeverityObservation FindingSeverity = "observation"
	SeverityMajor       FindingSeverity = "major"
	SeverityCritical    FindingSeverity = "critical"
)

type Inspection struct {
	ID            string           `json:"id"`
	WorkPackageID string           `json:"work_package_id"`
	InspectorID   string           `json:"inspector_id"`
	Checklist     string           `json:"checklist"`
	Status        InspectionStatus `json:"status"`
	ScheduledAt   time.Time        `json:"scheduled_at"`
	StartedAt     time.Time        `json:"started_at,omitempty"`
	CompletedAt   time.Time        `json:"completed_at,omitempty"`
	Version       int64            `json:"version"`
}

func (i *Inspection) Start(now time.Time) error {
	if i.Status != InspectionScheduled {
		return &StateError{Entity: "inspection", From: string(i.Status), To: string(InspectionExecuting), Reason: "only scheduled inspections can start"}
	}
	i.Status, i.StartedAt, i.Version = InspectionExecuting, now.UTC(), i.Version+1
	return nil
}

func (i *Inspection) Complete(passed bool, unresolved int, now time.Time) error {
	if i.Status != InspectionExecuting {
		return &StateError{Entity: "inspection", From: string(i.Status), To: "complete", Reason: "inspection is not executing"}
	}
	if passed && unresolved > 0 {
		return &StateError{Entity: "inspection", From: string(i.Status), To: string(InspectionPassed), Reason: "unresolved findings remain"}
	}
	if passed {
		i.Status = InspectionPassed
	} else {
		i.Status = InspectionFailed
	}
	i.CompletedAt, i.Version = now.UTC(), i.Version+1
	return nil
}

type Finding struct {
	ID           string          `json:"id"`
	InspectionID string          `json:"inspection_id"`
	Severity     FindingSeverity `json:"severity"`
	Summary      string          `json:"summary"`
	DueAt        time.Time       `json:"due_at"`
	ResolvedAt   time.Time       `json:"resolved_at,omitempty"`
	Resolution   string          `json:"resolution,omitempty"`
	Version      int64           `json:"version"`
}

func (f Finding) Validate() error {
	if err := Required("summary", f.Summary); err != nil {
		return err
	}
	switch f.Severity {
	case SeverityObservation, SeverityMajor, SeverityCritical:
	default:
		return &FieldError{Field: "severity", Reason: "unsupported finding severity"}
	}
	if f.DueAt.IsZero() {
		return &FieldError{Field: "due_at", Reason: "is required"}
	}
	return nil
}

func (f *Finding) Resolve(resolution string, now time.Time) error {
	if !f.ResolvedAt.IsZero() {
		return &StateError{Entity: "finding", From: "resolved", To: "resolved", Reason: "finding was already closed"}
	}
	if err := Required("resolution", resolution); err != nil {
		return err
	}
	f.Resolution, f.ResolvedAt, f.Version = resolution, now.UTC(), f.Version+1
	return nil
}
