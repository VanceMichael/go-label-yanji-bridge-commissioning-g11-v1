package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

type pausedReadingStore struct {
	repository.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *pausedReadingStore) AppendReading(ctx context.Context, reading domain.SensorReading) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.Store.AppendReading(ctx, reading)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEvaluationCannotPassBeforeConcurrentReadingAdmissionSettles(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 20, 4, 0, 0, 0, time.UTC)
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "bridgewatch.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Bootstrap(ctx, "org-1", "Bridge Commissioning", []sqlite.BootstrapUser{
		{ID: "commissioning-1", Email: "commissioning@example.test", DisplayName: "Commissioning", Password: "private-test-password", Role: domain.RoleCommissioning},
		{ID: "supervisor-1", Email: "supervisor@example.test", DisplayName: "Supervisor", Password: "private-test-password", Role: domain.RoleSupervisor},
	}, now); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	project := domain.Project{ID: "project-1", Organization: "org-1", Name: "North Span", Status: domain.ProjectTesting, TargetOpenAt: now.Add(30 * 24 * time.Hour), Timezone: "Asia/Shanghai", Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	plan := domain.LoadTestPlan{ID: "plan-1", ProjectID: project.ID, Name: "Static Load", Status: domain.LoadPlanApproved, ApprovedBy: "supervisor-1", ApprovedAt: now, Version: 2, CreatedAt: now}
	loadCase := domain.LoadCase{ID: "case-1", PlanID: plan.ID, Sequence: 1, Name: "Mid-span", TargetTonnes: 720, HoldSeconds: 300}
	channel := domain.SensorChannel{ID: "channel-1", PlanID: plan.ID, Code: "DEFLECT-MID", Unit: "mm", MinValue: -8, MaxValue: 8, Mandatory: true}
	if err := store.CreateLoadPlan(ctx, plan, []domain.LoadCase{loadCase}, []domain.SensorChannel{channel}); err != nil {
		t.Fatalf("create load plan: %v", err)
	}
	run := domain.LoadTestRun{ID: "run-1", PlanID: plan.ID, Status: domain.LoadRunRunning, StartedBy: "commissioning-1", StartedAt: now, Version: 2}
	if err := store.CreateLoadRun(ctx, run); err != nil {
		t.Fatalf("create load run: %v", err)
	}
	if err := store.AppendReading(ctx, domain.SensorReading{RunID: run.ID, ChannelID: channel.ID, Sequence: 1, Value: 2.4, ObservedAt: now}); err != nil {
		t.Fatalf("append in-range reading: %v", err)
	}

	paused := &pausedReadingStore{Store: store, entered: make(chan struct{}), release: make(chan struct{})}
	svc := New(paused, repository.FixedClock{Time: now.Add(time.Minute)})
	principal := domain.Principal{UserID: "commissioning-1", Organization: "org-1", Role: domain.RoleCommissioning}
	appendResult := make(chan error, 1)
	go func() {
		appendResult <- svc.AppendReading(ctx, principal, run.ID, domain.SensorReading{ChannelID: channel.ID, Sequence: 2, Value: 12.5, ObservedAt: now.Add(time.Second)})
	}()
	<-paused.entered

	if _, err := svc.QueueLoadEvaluation(ctx, principal, run.ID); err != nil {
		t.Fatalf("queue evaluation: %v", err)
	}
	if err := svc.EvaluateLoadRun(ctx, run.ID); err != nil {
		t.Fatalf("evaluate load run: %v", err)
	}
	terminal, err := store.GetLoadRun(ctx, "org-1", run.ID)
	if err != nil {
		t.Fatalf("get terminal run: %v", err)
	}
	if terminal.Status != domain.LoadRunPassed {
		t.Fatalf("terminal status before late reading = %s, want passed", terminal.Status)
	}

	close(paused.release)
	if err := <-appendResult; err != nil {
		t.Fatalf("late out-of-range reading was rejected: %v", err)
	}
	terminal, err = store.GetLoadRun(ctx, "org-1", run.ID)
	if err != nil {
		t.Fatalf("get run after late reading: %v", err)
	}
	passed, failures, err := store.EvaluateRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("re-evaluate durable readings: %v", err)
	}
	if terminal.Status == domain.LoadRunPassed && !passed && failures == 1 {
		t.Fatalf("passed run contains a durable out-of-range reading: status=%s passed=%v failures=%d", terminal.Status, passed, failures)
	}
}
