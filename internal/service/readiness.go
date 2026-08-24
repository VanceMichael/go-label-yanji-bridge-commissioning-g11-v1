package service

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

type OpeningReadiness struct {
	ProjectID string   `json:"project_id"`
	Ready     bool     `json:"ready"`
	Blockers  []string `json:"blockers"`
}

func (s *Service) OpeningReadiness(ctx context.Context, p domain.Principal, projectID string) (OpeningReadiness, error) {
	if _, err := s.store.GetProject(ctx, p.Organization, projectID); err != nil {
		return OpeningReadiness{}, fmt.Errorf("read readiness project: %w", err)
	}
	blockers, err := s.store.OpeningBlockers(ctx, projectID, s.clock.Now())
	if err != nil {
		return OpeningReadiness{}, fmt.Errorf("read opening blockers: %w", err)
	}
	return OpeningReadiness{ProjectID: projectID, Ready: len(blockers) == 0, Blockers: blockers}, nil
}
