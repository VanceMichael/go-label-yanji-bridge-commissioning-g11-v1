package domain

import "time"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobLeased    JobStatus = "leased"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Payload     []byte    `json:"payload"`
	Status      JobStatus `json:"status"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	AvailableAt time.Time `json:"available_at"`
	LeaseOwner  string    `json:"lease_owner,omitempty"`
	LeaseUntil  time.Time `json:"lease_until,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (j Job) Retryable() bool { return j.Attempts < j.MaxAttempts }

type AuditEvent struct {
	ID           string    `json:"id"`
	Organization string    `json:"organization_id"`
	ActorID      string    `json:"actor_id"`
	RequestID    string    `json:"request_id"`
	ObjectType   string    `json:"object_type"`
	ObjectID     string    `json:"object_id"`
	Action       string    `json:"action"`
	Result       string    `json:"result"`
	Detail       string    `json:"detail"`
	CreatedAt    time.Time `json:"created_at"`
}
