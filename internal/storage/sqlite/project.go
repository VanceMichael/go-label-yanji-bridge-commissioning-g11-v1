package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

func (d *DB) CreateProject(ctx context.Context, p domain.Project) error {
	return createProject(ctx, d.queryer(), p)
}
func (t *txRepo) CreateProject(ctx context.Context, p domain.Project) error {
	return createProject(ctx, t.queryer(), p)
}
func createProject(ctx context.Context, q querier, p domain.Project) error {
	_, err := q.ExecContext(ctx, `INSERT INTO projects(id,organization_id,name,status,target_open_at,timezone,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Organization, p.Name, p.Status, timestamp(p.TargetOpenAt), p.Timezone, p.Version, timestamp(p.CreatedAt), timestamp(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (d *DB) GetProject(ctx context.Context, organization, id string) (domain.Project, error) {
	return getProject(ctx, d.queryer(), organization, id)
}
func (t *txRepo) GetProject(ctx context.Context, organization, id string) (domain.Project, error) {
	return getProject(ctx, t.queryer(), organization, id)
}
func getProject(ctx context.Context, q querier, organization, id string) (domain.Project, error) {
	return scanProject(q.QueryRowContext(ctx, `SELECT id,organization_id,name,status,target_open_at,timezone,version,created_at,updated_at FROM projects WHERE organization_id=? AND id=?`, organization, id))
}

func scanProject(row scanner) (domain.Project, error) {
	var p domain.Project
	var target, created, updated string
	if err := row.Scan(&p.ID, &p.Organization, &p.Name, &p.Status, &target, &p.Timezone, &p.Version, &created, &updated); err != nil {
		return domain.Project{}, mapNotFound(err)
	}
	var err error
	if p.TargetOpenAt, err = parseTime(target); err != nil {
		return domain.Project{}, err
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return domain.Project{}, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

func (d *DB) UpdateProject(ctx context.Context, p domain.Project, expected int64) error {
	return updateProject(ctx, d.queryer(), p, expected)
}
func (t *txRepo) UpdateProject(ctx context.Context, p domain.Project, expected int64) error {
	return updateProject(ctx, t.queryer(), p, expected)
}
func updateProject(ctx context.Context, q querier, p domain.Project, expected int64) error {
	result, err := q.ExecContext(ctx, `UPDATE projects SET status=?,target_open_at=?,version=?,updated_at=? WHERE id=? AND organization_id=? AND version=?`, p.Status, timestamp(p.TargetOpenAt), p.Version, timestamp(p.UpdatedAt), p.ID, p.Organization, expected)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return requireAffected(result, "project", p.ID, expected)
}

func (d *DB) ListProjects(ctx context.Context, organization string, page repository.Page) ([]domain.Project, int, error) {
	return listProjects(ctx, d.queryer(), organization, page)
}
func (t *txRepo) ListProjects(ctx context.Context, organization string, page repository.Page) ([]domain.Project, int, error) {
	return listProjects(ctx, t.queryer(), organization, page)
}
func listProjects(ctx context.Context, q querier, organization string, page repository.Page) ([]domain.Project, int, error) {
	page = page.Normalize()
	where, args := `organization_id=?`, []any{organization}
	if page.Status != "" {
		where += ` AND status=?`
		args = append(args, page.Status)
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}
	order := "created_at"
	if page.Sort == "target_open_at" {
		order = "target_open_at"
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := q.QueryContext(ctx, `SELECT id,organization_id,name,status,target_open_at,timezone,version,created_at,updated_at FROM projects WHERE `+where+` ORDER BY `+order+` ASC,id ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	projects := make([]domain.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, total, nil
}

var _ = strings.Builder{}
