package service

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
	_ "modernc.org/sqlite"
)

type dossierLockingStore struct {
	repository.Store
	mu              sync.Mutex
	txCalls         int
	outsideRead     chan struct{}
	firstTxStarted  chan struct{}
	releaseFirstTx  chan struct{}
	outsideReadOnce sync.Once
	firstTxOnce     sync.Once
}

func (s *dossierLockingStore) GetDossier(ctx context.Context, organization, project, id string) (domain.HandoverDossier, error) {
	dossier, err := s.Store.GetDossier(ctx, organization, project, id)
	s.outsideReadOnce.Do(func() { close(s.outsideRead) })
	return dossier, err
}

func (s *dossierLockingStore) WithinTx(ctx context.Context, fn func(repository.MutationRepository) error) error {
	s.mu.Lock()
	s.txCalls++
	first := s.txCalls == 1
	s.mu.Unlock()
	if first {
		s.firstTxOnce.Do(func() { close(s.firstTxStarted) })
		<-s.releaseFirstTx
	}
	return s.Store.WithinTx(ctx, fn)
}

func TestSubmissionLocksConcurrentDocumentReplacement(t *testing.T) {
	ctx := context.Background()
	clock := repository.FixedClock{Time: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)}
	dbPath := filepath.Join(t.TempDir(), "bridgewatch.db")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx, "org-test", "Test Organization", []sqlite.BootstrapUser{
		{ID: "owner", Email: "owner@example.test", DisplayName: "Owner", Role: domain.RoleOwnerAdmin, Password: "BridgeWatch!2026"},
		{ID: "contractor", Email: "contractor@example.test", DisplayName: "Contractor", Role: domain.RoleContractorEngineer, Password: "BridgeWatch!2026"},
		{ID: "supervisor", Email: "supervisor@example.test", DisplayName: "Supervisor", Role: domain.RoleSupervisor, Password: "BridgeWatch!2026"},
		{ID: "commissioning", Email: "commissioning@example.test", DisplayName: "Commissioning", Role: domain.RoleCommissioning, Password: "BridgeWatch!2026"},
	}, clock.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	owner := domain.Principal{UserID: "owner", Organization: "org-test", Role: domain.RoleOwnerAdmin}
	contractor := domain.Principal{UserID: "contractor", Organization: "org-test", Role: domain.RoleContractorEngineer}
	supervisor := domain.Principal{UserID: "supervisor", Organization: "org-test", Role: domain.RoleSupervisor}
	commissioning := domain.Principal{UserID: "commissioning", Organization: "org-test", Role: domain.RoleCommissioning}
	base := New(store, clock)
	project, err := base.CreateProject(ctx, owner, "request-1", "", CreateProjectInput{
		Name: "Concurrent handover project", TargetOpenAt: clock.Now().Add(90 * 24 * time.Hour), Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	plan, err := base.CreateLoadPlan(ctx, commissioning, CreateLoadPlanInput{
		ProjectID: project.ID,
		Name:      "Handover load plan",
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
	if err := base.AppendReading(ctx, commissioning, run.ID, domain.SensorReading{ChannelID: plan.Channels[0].ID, Sequence: 1, Value: 1, ObservedAt: clock.Now()}); err != nil {
		t.Fatalf("AppendReading() error = %v", err)
	}
	if _, err := base.QueueLoadEvaluation(ctx, commissioning, run.ID); err != nil {
		t.Fatalf("QueueLoadEvaluation() error = %v", err)
	}
	if err := base.EvaluateLoadRun(ctx, run.ID); err != nil {
		t.Fatalf("EvaluateLoadRun() error = %v", err)
	}
	dossier, err := base.CreateDossier(ctx, owner, project.ID, []string{"completion-certificate"})
	if err != nil {
		t.Fatalf("CreateDossier() error = %v", err)
	}
	if err := base.ReceiveDossierDocument(ctx, owner, project.ID, dossier.ID, "completion-certificate", "bridgewatch://submitted"); err != nil {
		t.Fatalf("initial ReceiveDossierDocument() error = %v", err)
	}

	coordinated := &dossierLockingStore{
		Store:          store,
		outsideRead:    make(chan struct{}),
		firstTxStarted: make(chan struct{}),
		releaseFirstTx: make(chan struct{}),
	}
	target := New(coordinated, clock)
	receiveErr := make(chan error, 1)
	go func() {
		receiveErr <- target.ReceiveDossierDocument(ctx, contractor, project.ID, dossier.ID, "completion-certificate", "bridgewatch://after-submit")
	}()

	select {
	case <-coordinated.outsideRead:
		select {
		case <-coordinated.firstTxStarted:
		case <-time.After(2 * time.Second):
			t.Fatal("document replacement did not reach its write transaction")
		}
	case <-coordinated.firstTxStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("document replacement did not reach a synchronization point")
	}

	submitErr := make(chan error, 1)
	go func() {
		_, err := target.SubmitDossier(ctx, owner, project.ID, dossier.ID, dossier.Version)
		submitErr <- err
	}()
	select {
	case err := <-submitErr:
		if err != nil {
			t.Fatalf("SubmitDossier() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SubmitDossier() did not complete while document write was paused")
	}
	close(coordinated.releaseFirstTx)
	if err := <-receiveErr; err == nil {
		t.Fatal("ReceiveDossierDocument() succeeded after dossier submission")
	} else if !errors.As(err, new(*domain.StateError)) {
		// The expected fixed behavior is a state rejection; any other error is also a failure.
		t.Fatalf("ReceiveDossierDocument() error = %v, want locked dossier rejection", err)
	}

	if got := readDocumentURI(t, dbPath, dossier.ID, "completion-certificate"); got != "bridgewatch://submitted" {
		t.Fatalf("document URI after submission = %q, want original submitted URI", got)
	}
	updated, err := store.GetDossier(ctx, owner.Organization, project.ID, dossier.ID)
	if err != nil {
		t.Fatalf("GetDossier() error = %v", err)
	}
	if updated.Status != domain.DossierReview {
		t.Fatalf("dossier status = %q, want under_review", updated.Status)
	}
}

func readDocumentURI(t *testing.T, dbPath, dossierID, kind string) string {
	t.Helper()
	dsn := "file:" + url.PathEscape(dbPath) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	var uri string
	if err := db.QueryRow(`SELECT uri FROM dossier_documents WHERE dossier_id=? AND kind=?`, dossierID, kind).Scan(&uri); err != nil {
		t.Fatalf("read dossier document URI: %v", err)
	}
	return uri
}
