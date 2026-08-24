package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

type failingEvaluationEnqueueStore struct {
	repository.Store
	fail bool
}

func (s *failingEvaluationEnqueueStore) EnqueueJobDetached(ctx context.Context, job domain.Job) error {
	if s.fail {
		return errors.New("job store unavailable")
	}
	return s.Store.EnqueueJobDetached(ctx, job)
}

func TestQueueEvaluationFailureDoesNotStrandRun(t *testing.T) {
	ctx := context.Background()
	clock := repository.FixedClock{Time: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)}
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "bridgewatch.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx, "org-test", "Test Organization", []sqlite.BootstrapUser{
		{ID: "owner", Email: "owner@example.test", DisplayName: "Owner", Role: domain.RoleOwnerAdmin, Password: "BridgeWatch!2026"},
		{ID: "supervisor", Email: "supervisor@example.test", DisplayName: "Supervisor", Role: domain.RoleSupervisor, Password: "BridgeWatch!2026"},
		{ID: "commissioning", Email: "commissioning@example.test", DisplayName: "Commissioning", Role: domain.RoleCommissioning, Password: "BridgeWatch!2026"},
	}, clock.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	owner := domain.Principal{UserID: "owner", Organization: "org-test", Role: domain.RoleOwnerAdmin}
	supervisor := domain.Principal{UserID: "supervisor", Organization: "org-test", Role: domain.RoleSupervisor}
	commissioning := domain.Principal{UserID: "commissioning", Organization: "org-test", Role: domain.RoleCommissioning}
	base := New(store, clock)
	project, err := base.CreateProject(ctx, owner, "request-1", "", CreateProjectInput{Name: "Outbox bridge", TargetOpenAt: clock.Now().Add(90 * 24 * time.Hour), Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	plan, err := base.CreateLoadPlan(ctx, commissioning, CreateLoadPlanInput{
		ProjectID: project.ID,
		Name:      "Evaluation plan",
		Cases:     []domain.LoadCase{{Name: "Design load", TargetTonnes: 100, HoldSeconds: 60}},
		Channels:  []domain.SensorChannel{{Code: "DEFLECTION", Unit: "mm", MinValue: -10, MaxValue: 10, Mandatory: true}},
	})
	if err != nil {
		t.Fatalf("CreateLoadPlan() error = %v", err)
	}
	approved, err := base.ApproveLoadPlan(ctx, supervisor, project.ID, plan.Plan.ID, plan.Plan.Version)
	if err != nil {
		t.Fatalf("ApproveLoadPlan() error = %v", err)
	}
	run, err := base.StartLoadRun(ctx, commissioning, project.ID, approved.ID)
	if err != nil {
		t.Fatalf("StartLoadRun() error = %v", err)
	}

	failing := &failingEvaluationEnqueueStore{Store: store, fail: true}
	if _, err := New(failing, clock).QueueLoadEvaluation(ctx, commissioning, run.ID); err == nil {
		t.Fatal("QueueLoadEvaluation() succeeded while durable job insertion failed")
	}
	persisted, err := store.GetLoadRun(ctx, owner.Organization, run.ID)
	if err != nil {
		t.Fatalf("GetLoadRun() error = %v", err)
	}
	if persisted.Status != domain.LoadRunRunning || persisted.Version != run.Version {
		t.Fatalf("run after failed queue = status %q version %d, want running version %d", persisted.Status, persisted.Version, run.Version)
	}
	if _, err := store.ClaimJob(ctx, "worker-1", clock.Now(), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ClaimJob() after failed queue error = %v, want no pending job", err)
	}

	failing.fail = false
	queued, err := New(failing, clock).QueueLoadEvaluation(ctx, commissioning, run.ID)
	if err != nil {
		t.Fatalf("QueueLoadEvaluation() retry error = %v", err)
	}
	if queued.Status != domain.LoadRunEvaluating {
		t.Fatalf("retry status = %q, want evaluating", queued.Status)
	}
	job, err := store.ClaimJob(ctx, "worker-1", clock.Now(), time.Minute)
	if err != nil {
		t.Fatalf("ClaimJob() after retry error = %v", err)
	}
	if job.Kind != "evaluate_load_run" {
		t.Fatalf("claimed job kind = %q", job.Kind)
	}
}
