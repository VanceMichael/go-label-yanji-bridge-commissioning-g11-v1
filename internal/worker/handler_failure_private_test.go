package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

func TestHandlerFailureCannotBeAcknowledgedAsCompleted(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	openStore := func(t *testing.T) *sqlite.DB {
		t.Helper()
		store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "worker-lifecycle.db"))
		if err != nil {
			t.Fatalf("open worker store: %v", err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Errorf("close worker store: %v", err)
			}
		})
		return store
	}
	newRunner := func(store repository.Store) *Runner {
		runner := New(store, "commissioning-worker", time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
		runner.clock = repository.FixedClock{Time: now}
		runner.lease = time.Minute
		return runner
	}

	t.Run("failed evaluation remains retryable", func(t *testing.T) {
		store := openStore(t)
		if err := store.Bootstrap(context.Background(), "org-private", "Bridge commissioning", []sqlite.BootstrapUser{
			{ID: "owner-private", Email: "owner@example.test", DisplayName: "Owner", Password: "private-password", Role: domain.RoleOwnerAdmin},
			{ID: "commissioning-private", Email: "commissioning@example.test", DisplayName: "Commissioning", Password: "private-password", Role: domain.RoleCommissioning},
		}, now); err != nil {
			t.Fatalf("bootstrap evaluation owner: %v", err)
		}
		project := domain.Project{ID: "project-private", Organization: "org-private", Name: "Yanji Bridge", Status: domain.ProjectTesting, TargetOpenAt: now.Add(90 * 24 * time.Hour), Timezone: "Asia/Shanghai", Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := store.CreateProject(context.Background(), project); err != nil {
			t.Fatalf("create evaluation project: %v", err)
		}
		plan := domain.LoadTestPlan{ID: "plan-private", ProjectID: project.ID, Name: "Acceptance load", Status: domain.LoadPlanApproved, ApprovedBy: "commissioning-private", ApprovedAt: now, Version: 2, CreatedAt: now}
		if err := store.CreateLoadPlan(context.Background(), plan, nil, nil); err != nil {
			t.Fatalf("create evaluation plan: %v", err)
		}
		run := domain.LoadTestRun{ID: "run-private", PlanID: plan.ID, Status: domain.LoadRunEvaluating, StartedBy: "commissioning-private", StartedAt: now.Add(-time.Hour), Version: 3}
		if err := store.CreateLoadRun(context.Background(), run); err != nil {
			t.Fatalf("create evaluating run: %v", err)
		}
		job := domain.Job{ID: "job-private-failure", Kind: "evaluate_load_run", Payload: []byte(`{"run_id":"run-private"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now}
		if err := store.EnqueueJob(context.Background(), job); err != nil {
			t.Fatalf("enqueue evaluation job: %v", err)
		}

		runner := newRunner(store)
		runner.Register("evaluate_load_run", func(context.Context, domain.Job) error {
			return errors.New("sensor service unavailable")
		})
		err := runner.once(context.Background())
		if err == nil || !strings.Contains(err.Error(), "sensor service unavailable") {
			t.Fatalf("worker error = %v, want sensor failure", err)
		}
		persistedRun, err := store.GetLoadRun(context.Background(), "org-private", run.ID)
		if err != nil {
			t.Fatalf("read evaluating run: %v", err)
		}
		if persistedRun.Status != domain.LoadRunEvaluating {
			t.Fatalf("load run status = %q, want evaluating", persistedRun.Status)
		}
		reclaimed, err := store.ClaimJob(context.Background(), "replacement-worker", now.Add(time.Hour), time.Minute)
		if err != nil {
			t.Fatalf("failed evaluation job cannot be reclaimed after backoff: %v", err)
		}
		if reclaimed.ID != job.ID || reclaimed.LastError != "sensor service unavailable" {
			t.Fatalf("reclaimed job = %+v, want retryable job with failure evidence", reclaimed)
		}
	})

	t.Run("successful evaluation is consumed once", func(t *testing.T) {
		store := openStore(t)
		job := domain.Job{ID: "job-private-success", Kind: "evaluate_load_run", Payload: []byte(`{"run_id":"run-success"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now}
		if err := store.EnqueueJob(context.Background(), job); err != nil {
			t.Fatalf("enqueue successful evaluation: %v", err)
		}
		var calls atomic.Int64
		runner := newRunner(store)
		runner.Register("evaluate_load_run", func(context.Context, domain.Job) error {
			calls.Add(1)
			return nil
		})
		if err := runner.once(context.Background()); err != nil {
			t.Fatalf("successful evaluation: %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("successful handler calls = %d, want 1", calls.Load())
		}
		if _, err := store.ClaimJob(context.Background(), "replacement-worker", now.Add(time.Hour), time.Minute); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("completed evaluation was claimable: %v", err)
		}
	})
}
