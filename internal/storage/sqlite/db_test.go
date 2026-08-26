package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

var sequence atomic.Int64

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bridgewatch.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	seedDatabase(t, store)
	return store
}

func seedDatabase(t *testing.T, store *DB) {
	t.Helper()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	suffix := sequence.Add(1)
	organization := fmt.Sprintf("org-%d", suffix)
	user := fmt.Sprintf("user-%d", suffix)
	if _, err := store.db.Exec(`INSERT INTO organizations(id,name,created_at) VALUES(?,?,?)`, organization, "Test organization "+organization, timestamp(now)); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO users(id,organization_id,email,display_name,role,password_hash,created_at) VALUES(?,?,?,?,?,?,?)`, user, organization, user+"@example.test", "Test User", domain.RoleOwnerAdmin, []byte("hash"), timestamp(now)); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO projects(id,organization_id,name,status,target_open_at,timezone,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "project-1", organization, "Yanji Bridge", domain.ProjectCloseout, timestamp(now.Add(90*24*time.Hour)), "Asia/Shanghai", 1, timestamp(now), timestamp(now)); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO work_packages(id,project_id,organization_id,code,title,scope,risk,status,owner_id,due_at,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, "work-1", "project-1", organization, "COAT-1", "Tower coating", "Complete protective coating", domain.RiskHigh, domain.WorkPlanned, user, timestamp(now.Add(30*24*time.Hour)), 1, timestamp(now), timestamp(now)); err != nil {
		t.Fatalf("insert work package: %v", err)
	}
}

func seedIdentity(t *testing.T, store *DB) (string, string) {
	t.Helper()
	var organization, user string
	if err := store.db.QueryRow(`SELECT organization_id,id FROM users LIMIT 1`).Scan(&organization, &user); err != nil {
		t.Fatalf("read seeded identity: %v", err)
	}
	return organization, user
}

func TestOpenAppliesMigrationsAndForeignKeys(t *testing.T) {
	store := openTestDB(t)
	var version int
	if err := store.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}
	var foreignKeys int
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	var journalMode string
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestMigrationIsRepeatableAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	ctx := context.Background()
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if _, err := first.db.Exec(`INSERT INTO organizations(id,name,created_at) VALUES('org-restart','Restart Organization','2026-08-24T00:00:00Z')`); err != nil {
		t.Fatalf("insert before restart: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer second.Close()
	var name string
	if err := second.db.QueryRow(`SELECT name FROM organizations WHERE id='org-restart'`).Scan(&name); err != nil {
		t.Fatalf("read data after restart: %v", err)
	}
	if name != "Restart Organization" {
		t.Fatalf("name after restart = %q", name)
	}
	var migrationCount int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration rows = %d, want 1", migrationCount)
	}
}

func TestTransactionRollsBackAllCrossEntityWrites(t *testing.T) {
	store := openTestDB(t)
	organization, user := seedIdentity(t, store)
	now := time.Now().UTC()
	err := store.WithinTx(context.Background(), func(repo repository.MutationRepository) error {
		work := domain.WorkPackage{ID: "work-rollback", ProjectID: "project-1", Organization: organization, Code: "ROLLBACK", Title: "Rollback work", Scope: "must disappear", Risk: domain.RiskMedium, Status: domain.WorkPlanned, OwnerID: user, DueAt: now.Add(time.Hour), Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkPackage(context.Background(), work); err != nil {
			return err
		}
		event := domain.AuditEvent{ID: "audit-rollback", Organization: organization, ActorID: user, RequestID: "request-rollback", ObjectType: "work_package", ObjectID: work.ID, Action: "create", Result: "success", Detail: `{}`, CreatedAt: now}
		if err := repo.AppendAudit(context.Background(), event); err != nil {
			return err
		}
		return errors.New("force rollback after all writes")
	})
	if err == nil || err.Error() != "force rollback after all writes" {
		t.Fatalf("WithinTx() error = %v", err)
	}
	for table, id := range map[string]string{"work_packages": "work-rollback", "audit_events": "audit-rollback"} {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id=?", table)
		if err := store.db.QueryRow(query, id).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d rows in %s", count, table)
		}
	}
}

func TestTransactionCommitsRelatedWrites(t *testing.T) {
	store := openTestDB(t)
	organization, user := seedIdentity(t, store)
	now := time.Now().UTC()
	err := store.WithinTx(context.Background(), func(repo repository.MutationRepository) error {
		work := domain.WorkPackage{ID: "work-commit", ProjectID: "project-1", Organization: organization, Code: "COMMIT", Title: "Committed work", Scope: "must persist", Risk: domain.RiskLow, Status: domain.WorkPlanned, OwnerID: user, DueAt: now.Add(time.Hour), Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkPackage(context.Background(), work); err != nil {
			return err
		}
		return repo.AppendAudit(context.Background(), domain.AuditEvent{ID: "audit-commit", Organization: organization, ActorID: user, RequestID: "request-commit", ObjectType: "work_package", ObjectID: work.ID, Action: "create", Result: "success", Detail: `{}`, CreatedAt: now})
	})
	if err != nil {
		t.Fatalf("WithinTx() error = %v", err)
	}
	for table, id := range map[string]string{"work_packages": "work-commit", "audit_events": "audit-commit"} {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id=?", table)
		if err := store.db.QueryRow(query, id).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("commit left %d rows in %s, want 1", count, table)
		}
	}
}

func TestOptimisticWorkUpdateAllowsOnlyOneConcurrentWinner(t *testing.T) {
	store := openTestDB(t)
	organization, _ := seedIdentity(t, store)
	ctx := context.Background()
	original, err := store.GetWorkPackage(ctx, organization, "work-1")
	if err != nil {
		t.Fatalf("GetWorkPackage() error = %v", err)
	}
	barrier := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, nextStatus := range []domain.WorkStatus{domain.WorkActive, domain.WorkActive} {
		workers.Add(1)
		go func(status domain.WorkStatus) {
			defer workers.Done()
			copy := original
			<-barrier
			if err := copy.Transition(status, time.Now()); err != nil {
				results <- err
				return
			}
			results <- store.UpdateWorkPackage(ctx, copy, original.Version)
		}(nextStatus)
	}
	close(barrier)
	workers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected update error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	current, err := store.GetWorkPackage(ctx, organization, "work-1")
	if err != nil {
		t.Fatalf("read winner: %v", err)
	}
	if current.Status != domain.WorkActive || current.Version != 2 {
		t.Fatalf("winner state = %+v", current)
	}
}

func TestIdempotencyScopeSeparatesTenantMethodAndPath(t *testing.T) {
	store := openTestDB(t)
	organization, _ := seedIdentity(t, store)
	now := time.Now().UTC()
	base := repository.IdempotencyScope{Organization: organization, Method: "POST", Path: "/v1/projects", Key: "same-key"}
	record := repository.IdempotencyRecord{Scope: base, RequestHash: "hash-a", StatusCode: 201, Response: []byte(`{"id":"project-a"}`), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := store.PutIdempotency(context.Background(), record); err != nil {
		t.Fatalf("PutIdempotency() error = %v", err)
	}
	loaded, err := store.GetIdempotency(context.Background(), base)
	if err != nil {
		t.Fatalf("GetIdempotency() error = %v", err)
	}
	if loaded.RequestHash != record.RequestHash || string(loaded.Response) != string(record.Response) || loaded.StatusCode != 201 {
		t.Fatalf("loaded record = %+v, want %+v", loaded, record)
	}
	otherScopes := []repository.IdempotencyScope{
		{Organization: organization, Method: "PUT", Path: base.Path, Key: base.Key},
		{Organization: organization, Method: base.Method, Path: "/v1/work-packages", Key: base.Key},
		{Organization: organization, Method: base.Method, Path: base.Path, Key: "other-key"},
	}
	for _, scope := range otherScopes {
		_, err := store.GetIdempotency(context.Background(), scope)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("scope %+v error = %v, want not found", scope, err)
		}
	}
}

func TestJobLeaseRetryCompletionAndExpiredLeaseRecovery(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC()
	job := domain.Job{ID: "job-1", Kind: "evaluate_load_run", Payload: []byte(`{"run_id":"run-1"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now}
	if err := store.EnqueueJob(context.Background(), job); err != nil {
		t.Fatalf("EnqueueJob() error = %v", err)
	}
	claimed, err := store.ClaimJob(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatalf("first ClaimJob() error = %v", err)
	}
	if claimed.Status != domain.JobLeased || claimed.LeaseOwner != "worker-a" || claimed.Attempts != 1 {
		t.Fatalf("first claim = %+v", claimed)
	}
	if _, err := store.ClaimJob(context.Background(), "worker-b", now, time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("claim active lease error = %v, want not found", err)
	}
	recovered, err := store.ClaimJob(context.Background(), "worker-b", now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("recover expired lease error = %v", err)
	}
	if recovered.LeaseOwner != "worker-b" || recovered.Attempts != 2 {
		t.Fatalf("recovered claim = %+v", recovered)
	}
	if err := store.RetryJob(context.Background(), recovered, "temporary sensor gap", now.Add(3*time.Minute)); err != nil {
		t.Fatalf("RetryJob() error = %v", err)
	}
	if _, err := store.ClaimJob(context.Background(), "worker-c", now.Add(2*time.Minute), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("claim before retry availability error = %v, want not found", err)
	}
	final, err := store.ClaimJob(context.Background(), "worker-c", now.Add(4*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("final ClaimJob() error = %v", err)
	}
	if final.Attempts != 3 || final.LeaseOwner != "worker-c" {
		t.Fatalf("final claim = %+v", final)
	}
	if err := store.CompleteJob(context.Background(), final.ID, "worker-c", now.Add(4*time.Minute)); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	if _, err := store.ClaimJob(context.Background(), "worker-d", now.Add(10*time.Minute), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("completed job was reclaimed: %v", err)
	}
}

func TestContextCancellationPreventsTransactionMutation(t *testing.T) {
	store := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := store.WithinTx(ctx, func(repository.MutationRepository) error {
		called = true
		return nil
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("WithinTx() error = %v, want canceled", err)
	}
	if called {
		t.Fatal("transaction callback ran after cancellation")
	}
}
