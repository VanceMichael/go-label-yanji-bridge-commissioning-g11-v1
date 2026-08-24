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

type CreateWorkInput struct {
	ProjectID string           `json:"project_id"`
	Code      string           `json:"code"`
	Title     string           `json:"title"`
	Scope     string           `json:"scope"`
	Risk      domain.RiskLevel `json:"risk"`
	OwnerID   string           `json:"owner_id"`
	DueAt     time.Time        `json:"due_at"`
}

func (s *Service) CreateWorkPackage(ctx context.Context, p domain.Principal, requestID string, input CreateWorkInput) (domain.WorkPackage, error) {
	if err := requireRole(p, domain.RoleOwnerAdmin, domain.RoleContractorEngineer); err != nil {
		return domain.WorkPackage{}, err
	}
	now := s.clock.Now()
	work := domain.WorkPackage{ID: idgen.New("wrk"), ProjectID: input.ProjectID, Organization: p.Organization, Code: input.Code, Title: input.Title, Scope: input.Scope, Risk: input.Risk, Status: domain.WorkPlanned, OwnerID: input.OwnerID, DueAt: input.DueAt, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := work.Validate(); err != nil {
		return work, err
	}
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		if _, err := repo.GetProject(ctx, p.Organization, input.ProjectID); err != nil {
			return err
		}
		if err := repo.CreateWorkPackage(ctx, work); err != nil {
			return err
		}
		event, err := audit.Event(idgen.New("aud"), p, requestID, "work_package", work.ID, "create", "success", audit.Detail{"risk": work.Risk}, now)
		if err != nil {
			return err
		}
		return repo.AppendAudit(ctx, event)
	})
	if err != nil {
		return domain.WorkPackage{}, fmt.Errorf("create work package transaction: %w", err)
	}
	return work, nil
}

func (s *Service) TransitionWork(ctx context.Context, p domain.Principal, requestID, id string, to domain.WorkStatus, expected int64) (domain.WorkPackage, error) {
	if err := requireRole(p, domain.RoleContractorEngineer, domain.RoleSupervisor); err != nil {
		return domain.WorkPackage{}, err
	}
	var work domain.WorkPackage
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		current, err := repo.GetWorkPackage(ctx, p.Organization, id)
		if err != nil {
			return err
		}
		if current.Version != expected {
			conflict := &domain.VersionConflict{Entity: "work package", ID: id, Version: expected}
			return fmt.Errorf("stale work package version: %s", conflict)
		}
		work = current
		if err := work.Transition(to, s.clock.Now()); err != nil {
			return err
		}
		if to == domain.WorkAccepted && p.Role != domain.RoleSupervisor {
			return domain.ErrForbidden
		}
		if to == domain.WorkAccepted {
			passed, open, err := repo.WorkAcceptanceEvidence(ctx, p.Organization, id)
			if err != nil {
				return err
			}
			if !passed || open > 0 {
				return &domain.StateError{Entity: "work package", From: string(current.Status), To: string(to), Reason: "a passed inspection with no unresolved findings is required"}
			}
		}
		if err := repo.UpdateWorkPackage(ctx, work, expected); err != nil {
			return err
		}
		event, err := audit.Event(idgen.New("aud"), p, requestID, "work_package", id, "transition", "success", audit.Detail{"from": current.Status, "to": to}, s.clock.Now())
		if err != nil {
			return err
		}
		return repo.AppendAudit(ctx, event)
	})
	if err != nil {
		return domain.WorkPackage{}, fmt.Errorf("transition work package: %w", err)
	}
	return work, nil
}

func (s *Service) ScheduleInspection(ctx context.Context, p domain.Principal, workID, inspector, checklist string, at time.Time) (domain.Inspection, error) {
	if err := requireRole(p, domain.RoleSupervisor); err != nil {
		return domain.Inspection{}, err
	}
	work, err := s.store.GetWorkPackage(ctx, p.Organization, workID)
	if err != nil {
		return domain.Inspection{}, err
	}
	if work.Status != domain.WorkSubmitted {
		return domain.Inspection{}, &domain.StateError{Entity: "work package", From: string(work.Status), To: "inspection", Reason: "work must be submitted"}
	}
	inspectorUser, err := s.store.FindUser(ctx, inspector)
	if err != nil {
		return domain.Inspection{}, err
	}
	if inspectorUser.Organization != p.Organization || inspectorUser.Role != domain.RoleSupervisor {
		return domain.Inspection{}, domain.ErrForbidden
	}
	if checklist == "" || at.IsZero() {
		return domain.Inspection{}, domain.ErrInvalid
	}
	inspection := domain.Inspection{ID: idgen.New("ins"), WorkPackageID: workID, InspectorID: inspector, Checklist: checklist, Status: domain.InspectionScheduled, ScheduledAt: at.UTC(), Version: 1}
	if err := s.store.CreateInspection(ctx, inspection); err != nil {
		return domain.Inspection{}, fmt.Errorf("schedule inspection: %w", err)
	}
	return inspection, nil
}

type CompleteInspectionResult struct {
	Inspection domain.Inspection `json:"inspection"`
	Findings   []domain.Finding  `json:"findings"`
}

func (s *Service) CompleteInspection(ctx context.Context, p domain.Principal, id string, passed bool, findings []domain.Finding) (CompleteInspectionResult, error) {
	if err := requireRole(p, domain.RoleSupervisor); err != nil {
		return CompleteInspectionResult{}, err
	}
	var result CompleteInspectionResult
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		inspection, err := repo.GetInspection(ctx, p.Organization, id)
		if err != nil {
			return err
		}
		expected := inspection.Version
		if inspection.Status == domain.InspectionScheduled {
			if err := inspection.Start(s.clock.Now()); err != nil {
				return err
			}
			if err := repo.UpdateInspection(ctx, inspection, expected); err != nil {
				return err
			}
			expected = inspection.Version
		}
		for _, finding := range findings {
			finding.ID = idgen.New("fnd")
			finding.InspectionID = id
			finding.Version = 1
			if err := finding.Validate(); err != nil {
				return err
			}
			if err := repo.CreateFinding(ctx, finding); err != nil {
				return err
			}
			result.Findings = append(result.Findings, finding)
		}
		open, err := repo.CountOpenFindings(ctx, id)
		if err != nil {
			return err
		}
		if err := inspection.Complete(passed, open, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.UpdateInspection(ctx, inspection, expected); err != nil {
			return err
		}
		result.Inspection = inspection
		return nil
	})
	if err != nil {
		return CompleteInspectionResult{}, fmt.Errorf("complete inspection transaction: %w", err)
	}
	return result, nil
}

func (s *Service) ResolveFinding(ctx context.Context, p domain.Principal, id, resolution string, expected int64) (domain.Finding, error) {
	if err := requireRole(p, domain.RoleContractorEngineer, domain.RoleSupervisor); err != nil {
		return domain.Finding{}, err
	}
	var result domain.Finding
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		finding, err := repo.GetFinding(ctx, p.Organization, id)
		if err != nil {
			return err
		}
		if finding.Version != expected {
			return &domain.VersionConflict{Entity: "finding", ID: id, Version: expected}
		}
		if err := finding.Resolve(resolution, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.ResolveFinding(ctx, finding, expected); err != nil {
			return err
		}
		result = finding
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("resolve finding: %w", err)
	}
	return result, nil
}
