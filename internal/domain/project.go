package domain

import "time"

type ProjectStatus string

const (
	ProjectCloseout ProjectStatus = "closeout"
	ProjectTesting  ProjectStatus = "load_testing"
	ProjectHandover ProjectStatus = "handover"
	ProjectReady    ProjectStatus = "ready_to_open"
)

type Project struct {
	ID           string        `json:"id"`
	Organization string        `json:"organization_id"`
	Name         string        `json:"name"`
	Status       ProjectStatus `json:"status"`
	TargetOpenAt time.Time     `json:"target_open_at"`
	Timezone     string        `json:"timezone"`
	Version      int64         `json:"version"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

func (p Project) Validate() error {
	if err := Required("name", p.Name); err != nil {
		return err
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return &FieldError{Field: "timezone", Reason: "must be an IANA timezone"}
	}
	if p.TargetOpenAt.IsZero() {
		return &FieldError{Field: "target_open_at", Reason: "is required"}
	}
	return nil
}

func (p *Project) Advance(to ProjectStatus, now time.Time) error {
	valid := (p.Status == ProjectCloseout && to == ProjectTesting) ||
		(p.Status == ProjectTesting && to == ProjectHandover) ||
		(p.Status == ProjectHandover && to == ProjectReady)
	if !valid {
		return &StateError{Entity: "project", From: string(p.Status), To: string(to), Reason: "phase order is fixed"}
	}
	p.Status = to
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}
