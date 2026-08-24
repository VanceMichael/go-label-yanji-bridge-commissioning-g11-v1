package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

func (d *DB) advanceLegacySchemaVersion(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("prepare migration table: %w", err)
	}
	var current int
	if err := d.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read legacy schema version: %w", err)
	}
	var legacyTables int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&legacyTables); err != nil {
		return fmt.Errorf("inspect legacy schema: %w", err)
	}
	if current == 0 && legacyTables != 0 {
		if _, err := d.db.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(1, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
			return fmt.Errorf("advance legacy schema version: %w", err)
		}
	}
	return nil
}

func (d *DB) migrate(ctx context.Context) error {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > schemaVersion {
		return fmt.Errorf("database schema %d is newer than supported %d", current, schemaVersion)
	}
	if current == 0 {
		for _, statement := range migrationOne {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply schema v1: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(1, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
			return fmt.Errorf("record schema v1: %w", err)
		}
	}
	if err := validateHistoricalData(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func validateHistoricalData(ctx context.Context, q querier) error {
	checks := []string{
		`SELECT COUNT(*) FROM users u LEFT JOIN organizations o ON o.id=u.organization_id WHERE o.id IS NULL`,
		`SELECT COUNT(*) FROM work_packages w LEFT JOIN projects p ON p.id=w.project_id WHERE p.id IS NULL`,
		`SELECT COUNT(*) FROM inspections i LEFT JOIN work_packages w ON w.id=i.work_package_id WHERE w.id IS NULL`,
	}
	for _, check := range checks {
		var count int
		if err := q.QueryRowContext(ctx, check).Scan(&count); err != nil {
			return fmt.Errorf("validate historical data: %w", err)
		}
		if count != 0 {
			return fmt.Errorf("historical data contains %d relationship conflicts", count)
		}
	}
	return nil
}

var migrationOne = []string{
	`CREATE TABLE IF NOT EXISTS organizations(id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY, organization_id TEXT NOT NULL REFERENCES organizations(id), email TEXT NOT NULL COLLATE NOCASE, display_name TEXT NOT NULL, role TEXT NOT NULL, password_hash BLOB NOT NULL, disabled_at TEXT, created_at TEXT NOT NULL, UNIQUE(organization_id,email))`,
	`CREATE INDEX IF NOT EXISTS users_email_idx ON users(email)`,
	`CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), organization_id TEXT NOT NULL REFERENCES organizations(id), token_hash TEXT NOT NULL UNIQUE, expires_at TEXT NOT NULL, revoked_at TEXT, created_at TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS sessions_lookup_idx ON sessions(token_hash,expires_at,revoked_at)`,
	`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY, organization_id TEXT NOT NULL REFERENCES organizations(id), name TEXT NOT NULL, status TEXT NOT NULL, target_open_at TEXT NOT NULL, timezone TEXT NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(organization_id,name))`,
	`CREATE INDEX IF NOT EXISTS projects_org_status_idx ON projects(organization_id,status,target_open_at)`,
	`CREATE TABLE IF NOT EXISTS work_packages(id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id), organization_id TEXT NOT NULL REFERENCES organizations(id), code TEXT NOT NULL, title TEXT NOT NULL, scope TEXT NOT NULL, risk TEXT NOT NULL, status TEXT NOT NULL, owner_id TEXT NOT NULL REFERENCES users(id), due_at TEXT NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(project_id,code))`,
	`CREATE INDEX IF NOT EXISTS work_project_status_idx ON work_packages(project_id,status,risk,due_at)`,
	`CREATE TABLE IF NOT EXISTS inspections(id TEXT PRIMARY KEY, work_package_id TEXT NOT NULL REFERENCES work_packages(id), inspector_id TEXT NOT NULL REFERENCES users(id), checklist TEXT NOT NULL, status TEXT NOT NULL, scheduled_at TEXT NOT NULL, started_at TEXT, completed_at TEXT, version INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS inspections_work_status_idx ON inspections(work_package_id,status)`,
	`CREATE TABLE IF NOT EXISTS findings(id TEXT PRIMARY KEY, inspection_id TEXT NOT NULL REFERENCES inspections(id), severity TEXT NOT NULL, summary TEXT NOT NULL, due_at TEXT NOT NULL, resolved_at TEXT, resolution TEXT, version INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS findings_inspection_open_idx ON findings(inspection_id,resolved_at,severity)`,
	`CREATE TABLE IF NOT EXISTS load_test_plans(id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id), name TEXT NOT NULL, status TEXT NOT NULL, approved_by TEXT REFERENCES users(id), approved_at TEXT, version INTEGER NOT NULL, created_at TEXT NOT NULL, UNIQUE(project_id,name))`,
	`CREATE TABLE IF NOT EXISTS load_cases(id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES load_test_plans(id) ON DELETE CASCADE, sequence INTEGER NOT NULL, name TEXT NOT NULL, target_tonnes REAL NOT NULL, hold_seconds INTEGER NOT NULL, UNIQUE(plan_id,sequence))`,
	`CREATE TABLE IF NOT EXISTS sensor_channels(id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES load_test_plans(id) ON DELETE CASCADE, code TEXT NOT NULL, unit TEXT NOT NULL, min_value REAL NOT NULL, max_value REAL NOT NULL, mandatory INTEGER NOT NULL, UNIQUE(plan_id,code), CHECK(min_value<=max_value))`,
	`CREATE TABLE IF NOT EXISTS load_test_runs(id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES load_test_plans(id), status TEXT NOT NULL, started_by TEXT NOT NULL REFERENCES users(id), started_at TEXT, completed_at TEXT, failure TEXT, version INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS load_runs_plan_status_idx ON load_test_runs(plan_id,status)`,
	`CREATE TABLE IF NOT EXISTS sensor_readings(run_id TEXT NOT NULL REFERENCES load_test_runs(id) ON DELETE CASCADE, channel_id TEXT NOT NULL REFERENCES sensor_channels(id), sequence INTEGER NOT NULL, value REAL NOT NULL, observed_at TEXT NOT NULL, PRIMARY KEY(run_id,channel_id,sequence))`,
	`CREATE TABLE IF NOT EXISTS handover_dossiers(id TEXT PRIMARY KEY, project_id TEXT NOT NULL UNIQUE REFERENCES projects(id), status TEXT NOT NULL, submitted_by TEXT REFERENCES users(id), submitted_at TEXT, decided_by TEXT REFERENCES users(id), decided_at TEXT, decision_note TEXT, version INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dossier_documents(id TEXT PRIMARY KEY, dossier_id TEXT NOT NULL REFERENCES handover_dossiers(id) ON DELETE CASCADE, kind TEXT NOT NULL, uri TEXT, received_at TEXT, UNIQUE(dossier_id,kind))`,
	`CREATE TABLE IF NOT EXISTS approvals(id TEXT PRIMARY KEY, dossier_id TEXT NOT NULL REFERENCES handover_dossiers(id), actor_id TEXT NOT NULL REFERENCES users(id), decision TEXT NOT NULL, note TEXT NOT NULL, created_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS idempotency_keys(organization_id TEXT NOT NULL REFERENCES organizations(id), method TEXT NOT NULL, path TEXT NOT NULL, key TEXT NOT NULL, request_hash TEXT NOT NULL, status_code INTEGER NOT NULL, response BLOB NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, PRIMARY KEY(organization_id,method,path,key))`,
	`CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY, kind TEXT NOT NULL, payload BLOB NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL, max_attempts INTEGER NOT NULL, available_at TEXT NOT NULL, lease_owner TEXT, lease_until TEXT, last_error TEXT, created_at TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS jobs_claim_idx ON jobs(status,available_at,lease_until)`,
	`CREATE TABLE IF NOT EXISTS audit_events(id TEXT PRIMARY KEY, organization_id TEXT NOT NULL REFERENCES organizations(id), actor_id TEXT NOT NULL REFERENCES users(id), request_id TEXT NOT NULL, object_type TEXT NOT NULL, object_id TEXT NOT NULL, action TEXT NOT NULL, result TEXT NOT NULL, detail TEXT NOT NULL, created_at TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS audit_object_idx ON audit_events(organization_id,object_type,object_id,created_at)`,
}
