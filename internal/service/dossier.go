package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/idgen"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

func (s *Service) CreateDossier(ctx context.Context, p domain.Principal, projectID string, requiredDocuments []string) (domain.HandoverDossier, error) {
	if err := requireRole(p, domain.RoleOwnerAdmin); err != nil {
		return domain.HandoverDossier{}, err
	}
	if len(requiredDocuments) == 0 {
		return domain.HandoverDossier{}, domain.ErrInvalid
	}
	seen := make(map[string]struct{}, len(requiredDocuments))
	for index, kind := range requiredDocuments {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			return domain.HandoverDossier{}, domain.ErrInvalid
		}
		if _, exists := seen[kind]; exists {
			return domain.HandoverDossier{}, domain.ErrConflict
		}
		seen[kind] = struct{}{}
		requiredDocuments[index] = kind
	}
	dossier := domain.HandoverDossier{ID: idgen.New("dos"), ProjectID: projectID, Status: domain.DossierAssembling, Version: 1}
	if err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		if _, err := repo.GetProject(ctx, p.Organization, projectID); err != nil {
			return err
		}
		return repo.CreateDossier(ctx, dossier, requiredDocuments)
	}); err != nil {
		return dossier, fmt.Errorf("create dossier: %w", err)
	}
	return dossier, nil
}

func (s *Service) ReceiveDossierDocument(ctx context.Context, p domain.Principal, projectID, dossierID, kind, uri string) error {
	if err := requireRole(p, domain.RoleOwnerAdmin, domain.RoleContractorEngineer); err != nil {
		return err
	}
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(uri) == "" {
		return domain.ErrInvalid
	}
	dossier, err := s.store.GetDossier(ctx, p.Organization, projectID, dossierID)
	if err != nil {
		return err
	}
	if dossier.Status != domain.DossierAssembling {
		return &domain.StateError{Entity: "dossier", From: string(dossier.Status), To: "receive document", Reason: "documents are locked after submission"}
	}
	return s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		return repo.ReceiveDossierDocument(ctx, p.Organization, dossierID, strings.TrimSpace(kind), strings.TrimSpace(uri), s.clock.Now())
	})
}

func (s *Service) SubmitDossier(ctx context.Context, p domain.Principal, projectID, dossierID string, expected int64) (domain.HandoverDossier, error) {
	if err := requireRole(p, domain.RoleOwnerAdmin); err != nil {
		return domain.HandoverDossier{}, err
	}
	var result domain.HandoverDossier
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		dossier, err := repo.GetDossier(ctx, p.Organization, projectID, dossierID)
		if err != nil {
			return err
		}
		if dossier.Version != expected {
			return &domain.VersionConflict{Entity: "dossier", ID: dossierID, Version: expected}
		}
		evidence, err := repo.DossierEvidence(ctx, dossierID)
		if err != nil {
			return err
		}
		if err := dossier.Submit(p.UserID, evidence, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.UpdateDossier(ctx, dossier, expected); err != nil {
			return err
		}
		result = dossier
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("submit dossier: %w", err)
	}
	return result, nil
}

func (s *Service) DecideDossier(ctx context.Context, p domain.Principal, projectID, dossierID, note string, approve bool, expected int64) (domain.HandoverDossier, error) {
	if err := requireRole(p, domain.RoleSupervisor); err != nil {
		return domain.HandoverDossier{}, err
	}
	var result domain.HandoverDossier
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		dossier, err := repo.GetDossier(ctx, p.Organization, projectID, dossierID)
		if err != nil {
			return err
		}
		if dossier.Version != expected {
			return &domain.VersionConflict{Entity: "dossier", ID: dossierID, Version: expected}
		}
		if approve {
			evidence, err := repo.DossierEvidence(ctx, dossierID)
			if err != nil {
				return err
			}
			if err := evidence.Ready(); err != nil {
				return err
			}
		}
		if err := dossier.Decide(p.UserID, note, approve, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.UpdateDossier(ctx, dossier, expected); err != nil {
			return err
		}
		result = dossier
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("decide dossier: %w", err)
	}
	return result, nil
}
