package repository

import (
	"context"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

type AuthRepository interface {
	FindUserByEmail(context.Context, string) (domain.User, error)
	FindUser(context.Context, string) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	FindSessionByHash(context.Context, string) (domain.Session, domain.User, error)
	RevokeSession(context.Context, string, time.Time) error
}

type ProjectRepository interface {
	CreateProject(context.Context, domain.Project) error
	GetProject(context.Context, string, string) (domain.Project, error)
	UpdateProject(context.Context, domain.Project, int64) error
	ListProjects(context.Context, string, Page) ([]domain.Project, int, error)
}

type CloseoutRepository interface {
	CreateWorkPackage(context.Context, domain.WorkPackage) error
	GetWorkPackage(context.Context, string, string) (domain.WorkPackage, error)
	UpdateWorkPackage(context.Context, domain.WorkPackage, int64) error
	CreateInspection(context.Context, domain.Inspection) error
	GetInspection(context.Context, string, string) (domain.Inspection, error)
	UpdateInspection(context.Context, domain.Inspection, int64) error
	CreateFinding(context.Context, domain.Finding) error
	GetFinding(context.Context, string, string) (domain.Finding, error)
	ResolveFinding(context.Context, domain.Finding, int64) error
	CountOpenFindings(context.Context, string) (int, error)
	WorkAcceptanceEvidence(context.Context, string, string) (bool, int, error)
}

type LoadTestRepository interface {
	CreateLoadPlan(context.Context, domain.LoadTestPlan, []domain.LoadCase, []domain.SensorChannel) error
	GetLoadPlan(context.Context, string, string, string) (domain.LoadTestPlan, error)
	UpdateLoadPlan(context.Context, domain.LoadTestPlan, int64) error
	CountPlanParts(context.Context, string) (int, int, error)
	CreateLoadRun(context.Context, domain.LoadTestRun) error
	GetLoadRun(context.Context, string, string) (domain.LoadTestRun, error)
	GetLoadRunForEvaluation(context.Context, string) (domain.LoadTestRun, error)
	UpdateLoadRun(context.Context, domain.LoadTestRun, int64) error
	GetChannel(context.Context, string, string) (domain.SensorChannel, error)
	AppendReading(context.Context, domain.SensorReading) error
	EvaluateRun(context.Context, string) (bool, int, error)
}

type DossierRepository interface {
	CreateDossier(context.Context, domain.HandoverDossier, []string) error
	GetDossier(context.Context, string, string, string) (domain.HandoverDossier, error)
	ReceiveDossierDocument(context.Context, string, string, string, string, time.Time) error
	DossierEvidence(context.Context, string) (domain.DossierEvidence, error)
	UpdateDossier(context.Context, domain.HandoverDossier, int64) error
	OpeningBlockers(context.Context, string, time.Time) ([]string, error)
}

type JobRepository interface {
	EnqueueJob(context.Context, domain.Job) error
	EnqueueJobDetached(context.Context, domain.Job) error
	ClaimJob(context.Context, string, time.Time, time.Duration) (domain.Job, error)
	CompleteJob(context.Context, string, string, time.Time) error
	RetryJob(context.Context, domain.Job, string, time.Time) error
	ReleaseLeases(context.Context, string, time.Time) error
}

type MutationRepository interface {
	AuthRepository
	ProjectRepository
	CloseoutRepository
	LoadTestRepository
	DossierRepository
	JobRepository
	AppendAudit(context.Context, domain.AuditEvent) error
	GetIdempotency(context.Context, IdempotencyScope) (IdempotencyRecord, error)
	PutIdempotency(context.Context, IdempotencyRecord) error
}

type Store interface {
	MutationRepository
	WithinTx(context.Context, func(MutationRepository) error) error
	Ping(context.Context) error
	Close() error
}
