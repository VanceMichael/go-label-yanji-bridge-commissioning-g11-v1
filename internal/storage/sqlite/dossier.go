package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func (d *DB) CreateDossier(ctx context.Context, x domain.HandoverDossier, documents []string) error {
	return createDossier(ctx, d.queryer(), x, documents)
}
func (t *txRepo) CreateDossier(ctx context.Context, x domain.HandoverDossier, documents []string) error {
	return createDossier(ctx, t.queryer(), x, documents)
}
func createDossier(ctx context.Context, q querier, x domain.HandoverDossier, documents []string) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO handover_dossiers(id,project_id,status,submitted_by,submitted_at,decided_by,decided_at,decision_note,version) VALUES(?,?,?,?,?,?,?,?,?)`, x.ID, x.ProjectID, x.Status, optionalText(x.SubmittedBy), timestamp(x.SubmittedAt), optionalText(x.DecidedBy), timestamp(x.DecidedAt), x.DecisionNote, x.Version); err != nil {
		return fmt.Errorf("create dossier: %w", err)
	}
	for i, kind := range documents {
		if _, err := q.ExecContext(ctx, `INSERT INTO dossier_documents(id,dossier_id,kind,uri,received_at) VALUES(?,?,?,?,NULL)`, fmt.Sprintf("%s-doc-%02d", x.ID, i+1), x.ID, kind, ""); err != nil {
			return fmt.Errorf("create dossier document: %w", err)
		}
	}
	return nil
}
func (d *DB) GetDossier(ctx context.Context, organization, project, id string) (domain.HandoverDossier, error) {
	return getDossier(ctx, d.queryer(), organization, project, id)
}
func (t *txRepo) GetDossier(ctx context.Context, organization, project, id string) (domain.HandoverDossier, error) {
	return getDossier(ctx, t.queryer(), organization, project, id)
}
func getDossier(ctx context.Context, q querier, organization, project, id string) (domain.HandoverDossier, error) {
	row := q.QueryRowContext(ctx, `SELECT d.id,d.project_id,d.status,COALESCE(d.submitted_by,''),COALESCE(d.submitted_at,''),COALESCE(d.decided_by,''),COALESCE(d.decided_at,''),COALESCE(d.decision_note,''),d.version FROM handover_dossiers d JOIN projects p ON p.id=d.project_id WHERE p.organization_id=? AND d.project_id=? AND d.id=?`, organization, project, id)
	var x domain.HandoverDossier
	var submitted, decided string
	if err := row.Scan(&x.ID, &x.ProjectID, &x.Status, &x.SubmittedBy, &submitted, &x.DecidedBy, &decided, &x.DecisionNote, &x.Version); err != nil {
		return x, mapNotFound(err)
	}
	var err error
	if x.SubmittedAt, err = parseTime(submitted); err != nil {
		return x, err
	}
	if x.DecidedAt, err = parseTime(decided); err != nil {
		return x, err
	}
	return x, nil
}
func (d *DB) ReceiveDossierDocument(ctx context.Context, organization, dossierID, kind, uri string, now time.Time) error {
	return receiveDossierDocument(ctx, d.queryer(), organization, dossierID, kind, uri, now)
}
func (t *txRepo) ReceiveDossierDocument(ctx context.Context, organization, dossierID, kind, uri string, now time.Time) error {
	return receiveDossierDocument(ctx, t.queryer(), organization, dossierID, kind, uri, now)
}
func receiveDossierDocument(ctx context.Context, q querier, organization, dossierID, kind, uri string, now time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE dossier_documents SET uri=?,received_at=? WHERE dossier_id=? AND kind=? AND EXISTS(SELECT 1 FROM handover_dossiers d JOIN projects p ON p.id=d.project_id WHERE d.id=dossier_documents.dossier_id AND p.organization_id=?)`, uri, timestamp(now), dossierID, kind, organization)
	if err != nil {
		return fmt.Errorf("receive dossier document: %w", err)
	}
	return requireAffected(result, "dossier document", dossierID+":"+kind, 0)
}
func (d *DB) DossierEvidence(ctx context.Context, id string) (domain.DossierEvidence, error) {
	return dossierEvidence(ctx, d.queryer(), id)
}
func (t *txRepo) DossierEvidence(ctx context.Context, id string) (domain.DossierEvidence, error) {
	return dossierEvidence(ctx, t.queryer(), id)
}
func dossierEvidence(ctx context.Context, q querier, id string) (domain.DossierEvidence, error) {
	query := `SELECT (SELECT COUNT(*) FROM dossier_documents WHERE dossier_id=d.id),(SELECT COUNT(*) FROM dossier_documents WHERE dossier_id=d.id AND received_at IS NOT NULL),(SELECT COUNT(*) FROM findings f JOIN inspections i ON i.id=f.inspection_id JOIN work_packages w ON w.id=i.work_package_id WHERE w.project_id=d.project_id AND f.resolved_at IS NULL),(SELECT COUNT(*) FROM work_packages w WHERE w.project_id=d.project_id AND w.status<>'accepted'),(SELECT COUNT(*) FROM work_packages w WHERE w.project_id=d.project_id AND EXISTS(SELECT 1 FROM inspections failed WHERE failed.work_package_id=w.id AND failed.status='failed') AND NOT EXISTS(SELECT 1 FROM inspections passed WHERE passed.work_package_id=w.id AND passed.status='passed')),(SELECT COUNT(*) FROM load_test_runs r JOIN load_test_plans p ON p.id=r.plan_id WHERE p.project_id=d.project_id AND r.status='passed') FROM handover_dossiers d WHERE d.id=?`
	var e domain.DossierEvidence
	if err := q.QueryRowContext(ctx, query, id).Scan(&e.RequiredDocuments, &e.PresentDocuments, &e.OpenFindings, &e.UnacceptedWork, &e.FailedInspections, &e.PassedLoadRuns); err != nil {
		return e, mapNotFound(err)
	}
	return e, nil
}
func (d *DB) UpdateDossier(ctx context.Context, x domain.HandoverDossier, expected int64) error {
	return updateDossier(ctx, d.queryer(), x, expected)
}
func (t *txRepo) UpdateDossier(ctx context.Context, x domain.HandoverDossier, expected int64) error {
	return updateDossier(ctx, t.queryer(), x, expected)
}
func updateDossier(ctx context.Context, q querier, x domain.HandoverDossier, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE handover_dossiers SET status=?,submitted_by=?,submitted_at=?,decided_by=?,decided_at=?,decision_note=?,version=? WHERE id=? AND version=?`, x.Status, optionalText(x.SubmittedBy), timestamp(x.SubmittedAt), optionalText(x.DecidedBy), timestamp(x.DecidedAt), x.DecisionNote, x.Version, x.ID, expected)
	if err != nil {
		return fmt.Errorf("update dossier: %w", err)
	}
	return requireAffected(result, "dossier", x.ID, expected)
}
func (d *DB) OpeningBlockers(ctx context.Context, project string, now time.Time) ([]string, error) {
	return openingBlockers(ctx, d.queryer(), project, now)
}
func (t *txRepo) OpeningBlockers(ctx context.Context, project string, now time.Time) ([]string, error) {
	return openingBlockers(ctx, t.queryer(), project, now)
}
func openingBlockers(ctx context.Context, q querier, project string, now time.Time) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT 'work:'||code FROM work_packages WHERE project_id=? AND status<>'accepted' AND (risk IN ('high','critical') OR due_at<?) UNION ALL SELECT 'finding:'||f.id FROM findings f JOIN inspections i ON i.id=f.inspection_id JOIN work_packages w ON w.id=i.work_package_id WHERE w.project_id=? AND f.resolved_at IS NULL UNION ALL SELECT 'dossier:missing' WHERE NOT EXISTS(SELECT 1 FROM handover_dossiers WHERE project_id=?) UNION ALL SELECT 'dossier:'||id FROM handover_dossiers WHERE project_id=? AND status<>'approved' ORDER BY 1`, project, timestamp(now), project, project, project)
	if err != nil {
		return nil, fmt.Errorf("query opening blockers: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var blocker string
		if err := rows.Scan(&blocker); err != nil {
			return nil, err
		}
		result = append(result, blocker)
	}
	return result, rows.Err()
}
