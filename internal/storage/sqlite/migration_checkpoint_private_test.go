package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func TestFailedMigrationDoesNotAdvanceDurableSchemaVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	foundation, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("create legacy fixture: %v", err)
	}
	if err := foundation.Close(); err != nil {
		t.Fatalf("close legacy fixture: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy fixture: %v", err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable fixture foreign keys: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM schema_migrations`); err != nil {
		t.Fatalf("clear legacy migration checkpoint: %v", err)
	}
	if _, err := raw.Exec(`DROP TABLE jobs`); err != nil {
		t.Fatalf("remove table pending migration: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO users(id,organization_id,email,display_name,role,password_hash,created_at) VALUES('legacy-orphan','missing-organization','legacy@example.test','Legacy Operator','owner',x'01','2026-08-24T00:00:00Z')`); err != nil {
		t.Fatalf("insert deterministic historical conflict: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close prepared legacy database: %v", err)
	}

	if opened, err := Open(ctx, path); err == nil {
		opened.Close()
		t.Fatal("Open() succeeded with an unresolved historical relationship conflict")
	} else if !strings.Contains(err.Error(), "historical data contains 1 relationship conflicts") {
		t.Fatalf("Open() error = %v, want historical relationship conflict", err)
	}

	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopen failed migration database: %v", err)
	}
	var version int
	if err := raw.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version after failed migration: %v", err)
	}
	if version != 0 {
		t.Errorf("schema version after failed migration = %d, want 0", version)
	}
	if _, err := raw.Exec(`DELETE FROM users WHERE id='legacy-orphan'`); err != nil {
		t.Fatalf("repair historical conflict: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close repaired legacy database: %v", err)
	}

	recovered, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() after repairing historical conflict: %v", err)
	}
	defer recovered.Close()
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	job := domain.Job{ID: "migration-recovery-job", Kind: "evaluate_load_run", Payload: []byte(`{"run_id":"run-recovered"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now}
	if err := recovered.EnqueueJob(ctx, job); err != nil {
		t.Errorf("EnqueueJob() after migration recovery error = %v", err)
	}
}
