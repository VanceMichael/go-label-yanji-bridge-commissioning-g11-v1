package domain

import "time"

type WorkStatus string
type RiskLevel string

const (
	WorkPlanned   WorkStatus = "planned"
	WorkActive    WorkStatus = "active"
	WorkSubmitted WorkStatus = "submitted"
	WorkRework    WorkStatus = "rework"
	WorkAccepted  WorkStatus = "accepted"

	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type WorkPackage struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	Organization string     `json:"organization_id"`
	Code         string     `json:"code"`
	Title        string     `json:"title"`
	Scope        string     `json:"scope"`
	Risk         RiskLevel  `json:"risk"`
	Status       WorkStatus `json:"status"`
	OwnerID      string     `json:"owner_id"`
	DueAt        time.Time  `json:"due_at"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (w WorkPackage) Validate() error {
	for field, value := range map[string]string{"code": w.Code, "title": w.Title, "scope": w.Scope, "owner_id": w.OwnerID} {
		if err := Required(field, value); err != nil {
			return err
		}
	}
	switch w.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return &FieldError{Field: "risk", Reason: "unsupported risk level"}
	}
	return nil
}

func (w *WorkPackage) Transition(to WorkStatus, now time.Time) error {
	valid := false
	switch w.Status {
	case WorkPlanned:
		valid = to == WorkActive
	case WorkActive:
		valid = to == WorkSubmitted
	case WorkSubmitted:
		valid = to == WorkAccepted || to == WorkRework
	case WorkRework:
		valid = to == WorkActive
	}
	if !valid {
		return &StateError{Entity: "work package", From: string(w.Status), To: string(to), Reason: "transition violates closeout lifecycle"}
	}
	w.Status = to
	w.Version++
	w.UpdatedAt = now.UTC()
	return nil
}

func (w WorkPackage) BlocksOpening(now time.Time) bool {
	if w.Status == WorkAccepted {
		return false
	}
	return w.Risk == RiskHigh || w.Risk == RiskCritical || now.After(w.DueAt)
}
