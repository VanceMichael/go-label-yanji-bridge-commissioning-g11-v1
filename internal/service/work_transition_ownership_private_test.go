package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

type synchronizedTransitionStore struct {
	repository.Store

	readMu      sync.Mutex
	readers     int
	readsReady  chan struct{}
	transaction sync.Mutex
}

func newSynchronizedTransitionStore(store repository.Store) *synchronizedTransitionStore {
	return &synchronizedTransitionStore{Store: store, readsReady: make(chan struct{})}
}

func (s *synchronizedTransitionStore) GetWorkPackage(ctx context.Context, organization, id string) (domain.WorkPackage, error) {
	work, err := s.Store.GetWorkPackage(ctx, organization, id)
	if err != nil {
		return domain.WorkPackage{}, err
	}
	s.readMu.Lock()
	s.readers++
	if s.readers == 2 {
		close(s.readsReady)
	}
	ready := s.readsReady
	s.readMu.Unlock()
	select {
	case <-ready:
		return work, nil
	case <-ctx.Done():
		return domain.WorkPackage{}, ctx.Err()
	}
}

func (s *synchronizedTransitionStore) WithinTx(ctx context.Context, fn func(repository.MutationRepository) error) error {
	s.transaction.Lock()
	defer s.transaction.Unlock()
	return s.Store.WithinTx(ctx, fn)
}

func TestConcurrentWorkTransitionsHaveOneVersionOwner(t *testing.T) {
	ctx := context.Background()
	clock := repository.FixedClock{Time: time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)}
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "transition-ownership.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	users := []sqlite.BootstrapUser{
		{ID: "owner", Email: "owner@example.test", DisplayName: "Owner", Role: domain.RoleOwnerAdmin, Password: "BridgeWatch!2026"},
		{ID: "contractor", Email: "contractor@example.test", DisplayName: "Contractor", Role: domain.RoleContractorEngineer, Password: "BridgeWatch!2026"},
		{ID: "supervisor", Email: "supervisor@example.test", DisplayName: "Supervisor", Role: domain.RoleSupervisor, Password: "BridgeWatch!2026"},
	}
	if err := store.Bootstrap(ctx, "org-transition", "Transition Test Organization", users, clock.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	owner := domain.Principal{UserID: "owner", Organization: "org-transition", Role: domain.RoleOwnerAdmin}
	contractor := domain.Principal{UserID: "contractor", Organization: "org-transition", Role: domain.RoleContractorEngineer}
	supervisor := domain.Principal{UserID: "supervisor", Organization: "org-transition", Role: domain.RoleSupervisor}
	base := service.New(store, clock)
	project, err := base.CreateProject(ctx, owner, "request-project", "project-key", service.CreateProjectInput{
		Name:         "Transition ownership bridge",
		TargetOpenAt: clock.Now().Add(120 * 24 * time.Hour),
		Timezone:     "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	createSubmittedWork := func(code string) domain.WorkPackage {
		t.Helper()
		work, err := base.CreateWorkPackage(ctx, contractor, "request-work-"+code, service.CreateWorkInput{
			ProjectID: project.ID,
			Code:      code,
			Title:     "Closure decision " + code,
			Scope:     "Verify one owner for each submitted transition",
			Risk:      domain.RiskHigh,
			OwnerID:   contractor.UserID,
			DueAt:     clock.Now().Add(30 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateWorkPackage(%s) error = %v", code, err)
		}
		work, err = base.TransitionWork(ctx, contractor, "request-active-"+code, work.ID, domain.WorkActive, work.Version)
		if err != nil {
			t.Fatalf("TransitionWork(%s, active) error = %v", code, err)
		}
		work, err = base.TransitionWork(ctx, contractor, "request-submit-"+code, work.ID, domain.WorkSubmitted, work.Version)
		if err != nil {
			t.Fatalf("TransitionWork(%s, submitted) error = %v", code, err)
		}
		return work
	}

	sequential := createSubmittedWork("SEQUENTIAL")
	sequential, err = base.TransitionWork(ctx, contractor, "request-rework-sequential", sequential.ID, domain.WorkRework, sequential.Version)
	if err != nil {
		t.Fatalf("sequential submitted-to-rework transition error = %v", err)
	}
	if sequential.Status != domain.WorkRework || sequential.Version != 4 {
		t.Fatalf("sequential transition = status %q version %d, want rework version 4", sequential.Status, sequential.Version)
	}

	contended := createSubmittedWork("CONTENDED")
	inspection, err := base.ScheduleInspection(ctx, supervisor, contended.ID, supervisor.UserID, "acceptance-checklist", clock.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ScheduleInspection() error = %v", err)
	}
	if _, err := base.CompleteInspection(ctx, supervisor, inspection.ID, true, nil); err != nil {
		t.Fatalf("CompleteInspection() error = %v", err)
	}

	target := service.New(newSynchronizedTransitionStore(store), clock)
	start := make(chan struct{})
	type outcome struct {
		name string
		err  error
	}
	results := make(chan outcome, 2)
	var workers sync.WaitGroup
	for _, transition := range []struct {
		name      string
		principal domain.Principal
		status    domain.WorkStatus
	}{
		{name: "supervisor acceptance", principal: supervisor, status: domain.WorkAccepted},
		{name: "contractor rework", principal: contractor, status: domain.WorkRework},
	} {
		transition := transition
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := target.TransitionWork(ctx, transition.principal, "request-"+transition.name, contended.ID, transition.status, contended.Version)
			results <- outcome{name: transition.name, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
		case errors.Is(result.err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("%s returned unexpected error: %v", result.name, result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent transition successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	stored, err := store.GetWorkPackage(ctx, contractor.Organization, contended.ID)
	if err != nil {
		t.Fatalf("GetWorkPackage() error = %v", err)
	}
	if stored.Version != contended.Version+1 {
		t.Fatalf("stored version = %d, want exactly one increment to %d", stored.Version, contended.Version+1)
	}
	if stored.Status != domain.WorkAccepted && stored.Status != domain.WorkRework {
		t.Fatalf("stored status = %q, want accepted or rework", stored.Status)
	}
}
