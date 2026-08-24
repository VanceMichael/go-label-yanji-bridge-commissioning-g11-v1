package sqlite

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func (d *DB) CreateWorkPackage(ctx context.Context, w domain.WorkPackage) error {
	return createWork(ctx, d.queryer(), w)
}
func (t *txRepo) CreateWorkPackage(ctx context.Context, w domain.WorkPackage) error {
	return createWork(ctx, t.queryer(), w)
}
func createWork(ctx context.Context, q querier, w domain.WorkPackage) error {
	_, err := q.ExecContext(ctx, `INSERT INTO work_packages(id,project_id,organization_id,code,title,scope,risk,status,owner_id,due_at,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, w.ID, w.ProjectID, w.Organization, w.Code, w.Title, w.Scope, w.Risk, w.Status, w.OwnerID, timestamp(w.DueAt), w.Version, timestamp(w.CreatedAt), timestamp(w.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create work package: %w", err)
	}
	return nil
}
func (d *DB) GetWorkPackage(ctx context.Context, organization, id string) (domain.WorkPackage, error) {
	return getWork(ctx, d.queryer(), organization, id)
}
func (t *txRepo) GetWorkPackage(ctx context.Context, organization, id string) (domain.WorkPackage, error) {
	return getWork(ctx, t.queryer(), organization, id)
}
func getWork(ctx context.Context, q querier, organization, id string) (domain.WorkPackage, error) {
	row := q.QueryRowContext(ctx, `SELECT id,project_id,organization_id,code,title,scope,risk,status,owner_id,due_at,version,created_at,updated_at FROM work_packages WHERE organization_id=? AND id=?`, organization, id)
	var w domain.WorkPackage
	var due, created, updated string
	if err := row.Scan(&w.ID, &w.ProjectID, &w.Organization, &w.Code, &w.Title, &w.Scope, &w.Risk, &w.Status, &w.OwnerID, &due, &w.Version, &created, &updated); err != nil {
		return domain.WorkPackage{}, mapNotFound(err)
	}
	var err error
	if w.DueAt, err = parseTime(due); err != nil {
		return domain.WorkPackage{}, err
	}
	if w.CreatedAt, err = parseTime(created); err != nil {
		return domain.WorkPackage{}, err
	}
	if w.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.WorkPackage{}, err
	}
	return w, nil
}
func (d *DB) UpdateWorkPackage(ctx context.Context, w domain.WorkPackage, expected int64) error {
	return updateWork(ctx, d.queryer(), w, expected)
}
func (t *txRepo) UpdateWorkPackage(ctx context.Context, w domain.WorkPackage, expected int64) error {
	return updateWork(ctx, t.queryer(), w, expected)
}
func updateWork(ctx context.Context, q querier, w domain.WorkPackage, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE work_packages SET status=?,version=?,updated_at=? WHERE id=? AND organization_id=? AND version=?`, w.Status, w.Version, timestamp(w.UpdatedAt), w.ID, w.Organization, expected)
	if err != nil {
		return fmt.Errorf("update work package: %w", err)
	}
	return requireAffected(result, "work package", w.ID, expected)
}

func (d *DB) PersistWorkPackage(ctx context.Context, w domain.WorkPackage) error {
	return persistWork(ctx, d.queryer(), w)
}
func (t *txRepo) PersistWorkPackage(ctx context.Context, w domain.WorkPackage) error {
	return persistWork(ctx, t.queryer(), w)
}
func persistWork(ctx context.Context, q querier, w domain.WorkPackage) error {
	result, err := q.ExecContext(ctx, `UPDATE work_packages SET status=?,version=?,updated_at=? WHERE id=? AND organization_id=?`, w.Status, w.Version, timestamp(w.UpdatedAt), w.ID, w.Organization)
	if err != nil {
		return fmt.Errorf("persist work package: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect persisted work package: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func (d *DB) CreateInspection(ctx context.Context, i domain.Inspection) error {
	return createInspection(ctx, d.queryer(), i)
}
func (t *txRepo) CreateInspection(ctx context.Context, i domain.Inspection) error {
	return createInspection(ctx, t.queryer(), i)
}
func createInspection(ctx context.Context, q querier, i domain.Inspection) error {
	_, err := q.ExecContext(ctx, `INSERT INTO inspections(id,work_package_id,inspector_id,checklist,status,scheduled_at,started_at,completed_at,version) VALUES(?,?,?,?,?,?,?,?,?)`, i.ID, i.WorkPackageID, i.InspectorID, i.Checklist, i.Status, timestamp(i.ScheduledAt), timestamp(i.StartedAt), timestamp(i.CompletedAt), i.Version)
	if err != nil {
		return fmt.Errorf("create inspection: %w", err)
	}
	return nil
}
func (d *DB) GetInspection(ctx context.Context, organization, id string) (domain.Inspection, error) {
	return getInspection(ctx, d.queryer(), organization, id)
}
func (t *txRepo) GetInspection(ctx context.Context, organization, id string) (domain.Inspection, error) {
	return getInspection(ctx, t.queryer(), organization, id)
}
func getInspection(ctx context.Context, q querier, organization, id string) (domain.Inspection, error) {
	row := q.QueryRowContext(ctx, `SELECT i.id,i.work_package_id,i.inspector_id,i.checklist,i.status,i.scheduled_at,COALESCE(i.started_at,''),COALESCE(i.completed_at,''),i.version FROM inspections i JOIN work_packages w ON w.id=i.work_package_id WHERE w.organization_id=? AND i.id=?`, organization, id)
	var i domain.Inspection
	var scheduled, started, completed string
	if err := row.Scan(&i.ID, &i.WorkPackageID, &i.InspectorID, &i.Checklist, &i.Status, &scheduled, &started, &completed, &i.Version); err != nil {
		return domain.Inspection{}, mapNotFound(err)
	}
	var err error
	if i.ScheduledAt, err = parseTime(scheduled); err != nil {
		return domain.Inspection{}, err
	}
	if i.StartedAt, err = parseTime(started); err != nil {
		return domain.Inspection{}, err
	}
	if i.CompletedAt, err = parseTime(completed); err != nil {
		return domain.Inspection{}, err
	}
	return i, nil
}
func (d *DB) UpdateInspection(ctx context.Context, i domain.Inspection, expected int64) error {
	return updateInspection(ctx, d.queryer(), i, expected)
}
func (t *txRepo) UpdateInspection(ctx context.Context, i domain.Inspection, expected int64) error {
	return updateInspection(ctx, t.queryer(), i, expected)
}
func updateInspection(ctx context.Context, q querier, i domain.Inspection, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE inspections SET status=?,started_at=?,completed_at=?,version=? WHERE id=? AND version=?`, i.Status, timestamp(i.StartedAt), timestamp(i.CompletedAt), i.Version, i.ID, expected)
	if err != nil {
		return fmt.Errorf("update inspection: %w", err)
	}
	return requireAffected(result, "inspection", i.ID, expected)
}
func (d *DB) CreateFinding(ctx context.Context, f domain.Finding) error {
	return createFinding(ctx, d.queryer(), f)
}
func (t *txRepo) CreateFinding(ctx context.Context, f domain.Finding) error {
	return createFinding(ctx, t.queryer(), f)
}
func createFinding(ctx context.Context, q querier, f domain.Finding) error {
	_, err := q.ExecContext(ctx, `INSERT INTO findings(id,inspection_id,severity,summary,due_at,resolved_at,resolution,version) VALUES(?,?,?,?,?,?,?,?)`, f.ID, f.InspectionID, f.Severity, f.Summary, timestamp(f.DueAt), timestamp(f.ResolvedAt), f.Resolution, f.Version)
	if err != nil {
		return fmt.Errorf("create finding: %w", err)
	}
	return nil
}
func (d *DB) GetFinding(ctx context.Context, organization, id string) (domain.Finding, error) {
	return getFinding(ctx, d.queryer(), organization, id)
}
func (t *txRepo) GetFinding(ctx context.Context, organization, id string) (domain.Finding, error) {
	return getFinding(ctx, t.queryer(), organization, id)
}
func getFinding(ctx context.Context, q querier, organization, id string) (domain.Finding, error) {
	row := q.QueryRowContext(ctx, `SELECT f.id,f.inspection_id,f.severity,f.summary,f.due_at,COALESCE(f.resolved_at,''),COALESCE(f.resolution,''),f.version FROM findings f JOIN inspections i ON i.id=f.inspection_id JOIN work_packages w ON w.id=i.work_package_id WHERE w.organization_id=? AND f.id=?`, organization, id)
	var finding domain.Finding
	var due, resolved string
	if err := row.Scan(&finding.ID, &finding.InspectionID, &finding.Severity, &finding.Summary, &due, &resolved, &finding.Resolution, &finding.Version); err != nil {
		return finding, mapNotFound(err)
	}
	var err error
	if finding.DueAt, err = parseTime(due); err != nil {
		return finding, err
	}
	if finding.ResolvedAt, err = parseTime(resolved); err != nil {
		return finding, err
	}
	return finding, nil
}
func (d *DB) ResolveFinding(ctx context.Context, f domain.Finding, expected int64) error {
	return resolveFinding(ctx, d.queryer(), f, expected)
}
func (t *txRepo) ResolveFinding(ctx context.Context, f domain.Finding, expected int64) error {
	return resolveFinding(ctx, t.queryer(), f, expected)
}
func resolveFinding(ctx context.Context, q querier, f domain.Finding, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE findings SET resolved_at=?,resolution=?,version=? WHERE id=? AND version=? AND resolved_at IS NULL`, timestamp(f.ResolvedAt), f.Resolution, f.Version, f.ID, expected)
	if err != nil {
		return fmt.Errorf("resolve finding: %w", err)
	}
	return requireAffected(result, "finding", f.ID, expected)
}
func (d *DB) CountOpenFindings(ctx context.Context, inspectionID string) (int, error) {
	return countOpen(ctx, d.queryer(), inspectionID)
}
func (t *txRepo) CountOpenFindings(ctx context.Context, inspectionID string) (int, error) {
	return countOpen(ctx, t.queryer(), inspectionID)
}
func countOpen(ctx context.Context, q querier, id string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM findings WHERE inspection_id=? AND resolved_at IS NULL`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open findings: %w", err)
	}
	return count, nil
}

func (d *DB) WorkAcceptanceEvidence(ctx context.Context, organization, workID string) (bool, int, error) {
	return workAcceptanceEvidence(ctx, d.queryer(), organization, workID)
}
func (t *txRepo) WorkAcceptanceEvidence(ctx context.Context, organization, workID string) (bool, int, error) {
	return workAcceptanceEvidence(ctx, t.queryer(), organization, workID)
}
func workAcceptanceEvidence(ctx context.Context, q querier, organization, workID string) (bool, int, error) {
	var exists, passed, open int
	query := `SELECT COUNT(*),
		EXISTS(SELECT 1 FROM inspections i WHERE i.work_package_id=w.id AND i.status='passed'),
		(SELECT COUNT(*) FROM findings f JOIN inspections i ON i.id=f.inspection_id WHERE i.work_package_id=w.id AND f.resolved_at IS NULL)
		FROM work_packages w WHERE w.organization_id=? AND w.id=?`
	if err := q.QueryRowContext(ctx, query, organization, workID).Scan(&exists, &passed, &open); err != nil {
		return false, 0, fmt.Errorf("read work acceptance evidence: %w", err)
	}
	if exists == 0 {
		return false, 0, domain.ErrNotFound
	}
	return passed == 1, open, nil
}
