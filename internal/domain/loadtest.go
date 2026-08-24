package domain

import (
	"math"
	"time"
)

type LoadPlanStatus string
type LoadRunStatus string

const (
	LoadPlanDraft    LoadPlanStatus = "draft"
	LoadPlanApproved LoadPlanStatus = "approved"
	LoadPlanArchived LoadPlanStatus = "archived"

	LoadRunPending    LoadRunStatus = "pending"
	LoadRunRunning    LoadRunStatus = "running"
	LoadRunEvaluating LoadRunStatus = "evaluating"
	LoadRunPassed     LoadRunStatus = "passed"
	LoadRunFailed     LoadRunStatus = "failed"
	LoadRunCanceled   LoadRunStatus = "canceled"
)

type LoadTestPlan struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	Name       string         `json:"name"`
	Status     LoadPlanStatus `json:"status"`
	ApprovedBy string         `json:"approved_by,omitempty"`
	ApprovedAt time.Time      `json:"approved_at,omitempty"`
	Version    int64          `json:"version"`
	CreatedAt  time.Time      `json:"created_at"`
}

func (p *LoadTestPlan) Approve(actor string, caseCount, channelCount int, now time.Time) error {
	if p.Status != LoadPlanDraft {
		return &StateError{Entity: "load plan", From: string(p.Status), To: string(LoadPlanApproved), Reason: "plan is not draft"}
	}
	if caseCount <= 0 || channelCount <= 0 {
		return &StateError{Entity: "load plan", From: string(p.Status), To: string(LoadPlanApproved), Reason: "cases and sensor channels are required"}
	}
	p.Status, p.ApprovedBy, p.ApprovedAt, p.Version = LoadPlanApproved, actor, now.UTC(), p.Version+1
	return nil
}

type LoadCase struct {
	ID           string  `json:"id"`
	PlanID       string  `json:"plan_id"`
	Sequence     int     `json:"sequence"`
	Name         string  `json:"name"`
	TargetTonnes float64 `json:"target_tonnes"`
	HoldSeconds  int     `json:"hold_seconds"`
}

type SensorChannel struct {
	ID        string  `json:"id"`
	PlanID    string  `json:"plan_id"`
	Code      string  `json:"code"`
	Unit      string  `json:"unit"`
	MinValue  float64 `json:"min_value"`
	MaxValue  float64 `json:"max_value"`
	Mandatory bool    `json:"mandatory"`
}

type LoadTestRun struct {
	ID          string        `json:"id"`
	PlanID      string        `json:"plan_id"`
	Status      LoadRunStatus `json:"status"`
	StartedBy   string        `json:"started_by"`
	StartedAt   time.Time     `json:"started_at,omitempty"`
	CompletedAt time.Time     `json:"completed_at,omitempty"`
	Failure     string        `json:"failure,omitempty"`
	Version     int64         `json:"version"`
}

func (r *LoadTestRun) Transition(to LoadRunStatus, now time.Time) error {
	valid := (r.Status == LoadRunPending && to == LoadRunRunning) ||
		(r.Status == LoadRunRunning && (to == LoadRunEvaluating || to == LoadRunCanceled)) ||
		(r.Status == LoadRunEvaluating && (to == LoadRunPassed || to == LoadRunFailed))
	if !valid {
		return &StateError{Entity: "load run", From: string(r.Status), To: string(to), Reason: "transition violates execution lifecycle"}
	}
	if to == LoadRunRunning {
		r.StartedAt = now.UTC()
	}
	if to == LoadRunPassed || to == LoadRunFailed || to == LoadRunCanceled {
		r.CompletedAt = now.UTC()
	}
	r.Status, r.Version = to, r.Version+1
	return nil
}

type SensorReading struct {
	RunID      string    `json:"run_id"`
	ChannelID  string    `json:"channel_id"`
	Sequence   int64     `json:"sequence"`
	Value      float64   `json:"value"`
	ObservedAt time.Time `json:"observed_at"`
}

func (r SensorReading) Validate(channel SensorChannel) error {
	if r.RunID == "" || r.ChannelID != channel.ID || r.Sequence < 1 || r.ObservedAt.IsZero() || math.IsNaN(r.Value) || math.IsInf(r.Value, 0) {
		return &FieldError{Field: "reading", Reason: "run, channel, sequence, timestamp and finite value are required"}
	}
	return nil
}

func (r SensorReading) Within(channel SensorChannel) bool {
	return r.Value >= channel.MinValue && r.Value <= channel.MaxValue
}
