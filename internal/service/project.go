package service

import (
	"context"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/idgen"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

type CreateProjectInput struct {
	Name         string    `json:"name"`
	TargetOpenAt time.Time `json:"target_open_at"`
	Timezone     string    `json:"timezone"`
}

func (s *Service) CreateProject(ctx context.Context, p domain.Principal, requestID, key string, input CreateProjectInput) (domain.Project, error) {
	if err := requireRole(p, domain.RoleOwnerAdmin); err != nil {
		return domain.Project{}, err
	}
	now := s.clock.Now()
	project := domain.Project{ID: idgen.New("prj"), Organization: p.Organization, Name: input.Name, Status: domain.ProjectCloseout, TargetOpenAt: input.TargetOpenAt, Timezone: input.Timezone, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := project.Validate(); err != nil {
		return domain.Project{}, err
	}
	hash, err := hashRequest(input)
	if err != nil {
		return domain.Project{}, err
	}
	scope := repository.IdempotencyScope{Organization: p.Organization, Method: "POST", Path: "/v1/projects", Key: key}
	if key != "" {
		var cached domain.Project
		found, _, err := replay(ctx, s.store, scope, hash, now, &cached)
		if err != nil {
			return domain.Project{}, err
		}
		if found {
			return cached, nil
		}
	}
	if err := s.store.CreateProject(ctx, project); err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}
	err = s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		event, err := audit.Event(idgen.New("aud"), p, requestID, "project", project.ID, "create", "success", audit.Detail{"status": project.Status}, now)
		if err != nil {
			return err
		}
		if err := repo.AppendAudit(ctx, event); err != nil {
			return err
		}
		if key != "" {
			return remember(ctx, repo, scope, hash, 201, project, now)
		}
		return nil
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("create project transaction: %w", err)
	}
	return project, nil
}

func (s *Service) GetProject(ctx context.Context, p domain.Principal, id string) (domain.Project, error) {
	project, err := s.store.GetProject(ctx, p.Organization, id)
	if err != nil {
		return project, fmt.Errorf("get project: %w", err)
	}
	return project, nil
}
func (s *Service) ListProjects(ctx context.Context, p domain.Principal, page repository.Page) ([]domain.Project, int, error) {
	items, total, err := s.store.ListProjects(ctx, p.Organization, page)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	return items, total, nil
}
