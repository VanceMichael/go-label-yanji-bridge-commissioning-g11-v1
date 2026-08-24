package sqlite

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func (d *DB) CreateLoadPlan(ctx context.Context, p domain.LoadTestPlan, cases []domain.LoadCase, channels []domain.SensorChannel) error {
	return createLoadPlan(ctx, d.queryer(), p, cases, channels)
}
func (t *txRepo) CreateLoadPlan(ctx context.Context, p domain.LoadTestPlan, cases []domain.LoadCase, channels []domain.SensorChannel) error {
	return createLoadPlan(ctx, t.queryer(), p, cases, channels)
}
func createLoadPlan(ctx context.Context, q querier, p domain.LoadTestPlan, cases []domain.LoadCase, channels []domain.SensorChannel) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO load_test_plans(id,project_id,name,status,approved_by,approved_at,version,created_at) VALUES(?,?,?,?,?,?,?,?)`, p.ID, p.ProjectID, p.Name, p.Status, optionalText(p.ApprovedBy), timestamp(p.ApprovedAt), p.Version, timestamp(p.CreatedAt)); err != nil {
		return fmt.Errorf("create load plan: %w", err)
	}
	for _, item := range cases {
		if _, err := q.ExecContext(ctx, `INSERT INTO load_cases(id,plan_id,sequence,name,target_tonnes,hold_seconds) VALUES(?,?,?,?,?,?)`, item.ID, p.ID, item.Sequence, item.Name, item.TargetTonnes, item.HoldSeconds); err != nil {
			return fmt.Errorf("create load case: %w", err)
		}
	}
	for _, channel := range channels {
		if _, err := q.ExecContext(ctx, `INSERT INTO sensor_channels(id,plan_id,code,unit,min_value,max_value,mandatory) VALUES(?,?,?,?,?,?,?)`, channel.ID, p.ID, channel.Code, channel.Unit, channel.MinValue, channel.MaxValue, channel.Mandatory); err != nil {
			return fmt.Errorf("create sensor channel: %w", err)
		}
	}
	return nil
}
func (d *DB) GetLoadPlan(ctx context.Context, organization, project, id string) (domain.LoadTestPlan, error) {
	return getLoadPlan(ctx, d.queryer(), organization, project, id)
}
func (t *txRepo) GetLoadPlan(ctx context.Context, organization, project, id string) (domain.LoadTestPlan, error) {
	return getLoadPlan(ctx, t.queryer(), organization, project, id)
}
func getLoadPlan(ctx context.Context, q querier, organization, project, id string) (domain.LoadTestPlan, error) {
	row := q.QueryRowContext(ctx, `SELECT p.id,p.project_id,p.name,p.status,COALESCE(p.approved_by,''),COALESCE(p.approved_at,''),p.version,p.created_at FROM load_test_plans p JOIN projects x ON x.id=p.project_id WHERE x.organization_id=? AND p.project_id=? AND p.id=?`, organization, project, id)
	var p domain.LoadTestPlan
	var approved, created string
	if err := row.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Status, &p.ApprovedBy, &approved, &p.Version, &created); err != nil {
		return domain.LoadTestPlan{}, mapNotFound(err)
	}
	var err error
	if p.ApprovedAt, err = parseTime(approved); err != nil {
		return p, err
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return p, err
	}
	return p, nil
}
func (d *DB) UpdateLoadPlan(ctx context.Context, p domain.LoadTestPlan, expected int64) error {
	return updateLoadPlan(ctx, d.queryer(), p, expected)
}
func (t *txRepo) UpdateLoadPlan(ctx context.Context, p domain.LoadTestPlan, expected int64) error {
	return updateLoadPlan(ctx, t.queryer(), p, expected)
}
func updateLoadPlan(ctx context.Context, q querier, p domain.LoadTestPlan, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE load_test_plans SET status=?,approved_by=?,approved_at=?,version=? WHERE id=? AND version=?`, p.Status, p.ApprovedBy, timestamp(p.ApprovedAt), p.Version, p.ID, expected)
	if err != nil {
		return fmt.Errorf("update load plan: %w", err)
	}
	return requireAffected(result, "load plan", p.ID, expected)
}
func (d *DB) CountPlanParts(ctx context.Context, id string) (int, int, error) {
	return countPlanParts(ctx, d.queryer(), id)
}
func (t *txRepo) CountPlanParts(ctx context.Context, id string) (int, int, error) {
	return countPlanParts(ctx, t.queryer(), id)
}
func countPlanParts(ctx context.Context, q querier, id string) (int, int, error) {
	var cases, channels int
	if err := q.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM load_cases WHERE plan_id=?),(SELECT COUNT(*) FROM sensor_channels WHERE plan_id=?)`, id, id).Scan(&cases, &channels); err != nil {
		return 0, 0, fmt.Errorf("count plan parts: %w", err)
	}
	return cases, channels, nil
}
func (d *DB) CreateLoadRun(ctx context.Context, r domain.LoadTestRun) error {
	return createLoadRun(ctx, d.queryer(), r)
}
func (t *txRepo) CreateLoadRun(ctx context.Context, r domain.LoadTestRun) error {
	return createLoadRun(ctx, t.queryer(), r)
}
func createLoadRun(ctx context.Context, q querier, r domain.LoadTestRun) error {
	_, err := q.ExecContext(ctx, `INSERT INTO load_test_runs(id,plan_id,status,started_by,started_at,completed_at,failure,version) VALUES(?,?,?,?,?,?,?,?)`, r.ID, r.PlanID, r.Status, r.StartedBy, timestamp(r.StartedAt), timestamp(r.CompletedAt), r.Failure, r.Version)
	if err != nil {
		return fmt.Errorf("create load run: %w", err)
	}
	return nil
}
func (d *DB) GetLoadRun(ctx context.Context, organization, id string) (domain.LoadTestRun, error) {
	return getLoadRun(ctx, d.queryer(), organization, id)
}
func (t *txRepo) GetLoadRun(ctx context.Context, organization, id string) (domain.LoadTestRun, error) {
	return getLoadRun(ctx, t.queryer(), organization, id)
}
func getLoadRun(ctx context.Context, q querier, organization, id string) (domain.LoadTestRun, error) {
	row := q.QueryRowContext(ctx, `SELECT r.id,r.plan_id,r.status,r.started_by,COALESCE(r.started_at,''),COALESCE(r.completed_at,''),COALESCE(r.failure,''),r.version FROM load_test_runs r JOIN load_test_plans p ON p.id=r.plan_id JOIN projects x ON x.id=p.project_id WHERE x.organization_id=? AND r.id=?`, organization, id)
	return scanLoadRun(row)
}
func (d *DB) GetLoadRunForEvaluation(ctx context.Context, id string) (domain.LoadTestRun, error) {
	return getLoadRunForEvaluation(ctx, d.queryer(), id)
}
func (t *txRepo) GetLoadRunForEvaluation(ctx context.Context, id string) (domain.LoadTestRun, error) {
	return getLoadRunForEvaluation(ctx, t.queryer(), id)
}
func getLoadRunForEvaluation(ctx context.Context, q querier, id string) (domain.LoadTestRun, error) {
	return scanLoadRun(q.QueryRowContext(ctx, `SELECT id,plan_id,status,started_by,COALESCE(started_at,''),COALESCE(completed_at,''),COALESCE(failure,''),version FROM load_test_runs WHERE id=?`, id))
}
func scanLoadRun(row scanner) (domain.LoadTestRun, error) {
	var r domain.LoadTestRun
	var started, completed string
	if err := row.Scan(&r.ID, &r.PlanID, &r.Status, &r.StartedBy, &started, &completed, &r.Failure, &r.Version); err != nil {
		return r, mapNotFound(err)
	}
	var err error
	if r.StartedAt, err = parseTime(started); err != nil {
		return r, err
	}
	if r.CompletedAt, err = parseTime(completed); err != nil {
		return r, err
	}
	return r, nil
}
func (d *DB) UpdateLoadRun(ctx context.Context, r domain.LoadTestRun, expected int64) error {
	return updateLoadRun(ctx, d.queryer(), r, expected)
}
func (t *txRepo) UpdateLoadRun(ctx context.Context, r domain.LoadTestRun, expected int64) error {
	return updateLoadRun(ctx, t.queryer(), r, expected)
}
func updateLoadRun(ctx context.Context, q querier, r domain.LoadTestRun, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE load_test_runs SET status=?,started_at=?,completed_at=?,failure=?,version=? WHERE id=? AND version=?`, r.Status, timestamp(r.StartedAt), timestamp(r.CompletedAt), r.Failure, r.Version, r.ID, expected)
	if err != nil {
		return fmt.Errorf("update load run: %w", err)
	}
	return requireAffected(result, "load run", r.ID, expected)
}
func (d *DB) GetChannel(ctx context.Context, plan, id string) (domain.SensorChannel, error) {
	return getChannel(ctx, d.queryer(), plan, id)
}
func (t *txRepo) GetChannel(ctx context.Context, plan, id string) (domain.SensorChannel, error) {
	return getChannel(ctx, t.queryer(), plan, id)
}
func getChannel(ctx context.Context, q querier, plan, id string) (domain.SensorChannel, error) {
	var c domain.SensorChannel
	if err := q.QueryRowContext(ctx, `SELECT id,plan_id,code,unit,min_value,max_value,mandatory FROM sensor_channels WHERE plan_id=? AND id=?`, plan, id).Scan(&c.ID, &c.PlanID, &c.Code, &c.Unit, &c.MinValue, &c.MaxValue, &c.Mandatory); err != nil {
		return c, mapNotFound(err)
	}
	return c, nil
}
func (d *DB) AppendReading(ctx context.Context, r domain.SensorReading) error {
	return appendReading(ctx, d.queryer(), r)
}
func (t *txRepo) AppendReading(ctx context.Context, r domain.SensorReading) error {
	return appendReading(ctx, t.queryer(), r)
}
func appendReading(ctx context.Context, q querier, r domain.SensorReading) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sensor_readings(run_id,channel_id,sequence,value,observed_at) VALUES(?,?,?,?,?)`, r.RunID, r.ChannelID, r.Sequence, r.Value, timestamp(r.ObservedAt))
	if err != nil {
		return fmt.Errorf("append sensor reading: %w", err)
	}
	return nil
}
func (d *DB) EvaluateRun(ctx context.Context, id string) (bool, int, error) {
	return evaluateRun(ctx, d.queryer(), id)
}
func (t *txRepo) EvaluateRun(ctx context.Context, id string) (bool, int, error) {
	return evaluateRun(ctx, t.queryer(), id)
}
func evaluateRun(ctx context.Context, q querier, id string) (bool, int, error) {
	var outOfRange, missing int
	query := `SELECT COUNT(*) FROM sensor_readings r JOIN sensor_channels c ON c.id=r.channel_id JOIN load_test_runs x ON x.id=r.run_id WHERE r.run_id=? AND (r.value<c.min_value OR r.value>c.max_value)`
	if err := q.QueryRowContext(ctx, query, id).Scan(&outOfRange); err != nil {
		return false, 0, fmt.Errorf("evaluate readings: %w", err)
	}
	query = `SELECT COUNT(*) FROM sensor_channels c JOIN load_test_runs x ON x.plan_id=c.plan_id WHERE x.id=? AND c.mandatory=1 AND NOT EXISTS(SELECT 1 FROM sensor_readings r WHERE r.run_id=x.id AND r.channel_id=c.id)`
	if err := q.QueryRowContext(ctx, query, id).Scan(&missing); err != nil {
		return false, 0, fmt.Errorf("evaluate mandatory channels: %w", err)
	}
	return outOfRange == 0 && missing == 0, outOfRange + missing, nil
}
