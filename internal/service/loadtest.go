package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/idgen"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

type CreateLoadPlanInput struct {
	ProjectID string                 `json:"project_id"`
	Name      string                 `json:"name"`
	Cases     []domain.LoadCase      `json:"cases"`
	Channels  []domain.SensorChannel `json:"channels"`
}

type CreateLoadPlanResult struct {
	Plan     domain.LoadTestPlan    `json:"plan"`
	Cases    []domain.LoadCase      `json:"cases"`
	Channels []domain.SensorChannel `json:"channels"`
}

func (s *Service) CreateLoadPlan(ctx context.Context, p domain.Principal, input CreateLoadPlanInput) (CreateLoadPlanResult, error) {
	if err := requireRole(p, domain.RoleCommissioning); err != nil {
		return CreateLoadPlanResult{}, err
	}
	if input.Name == "" || len(input.Cases) == 0 || len(input.Channels) == 0 {
		return CreateLoadPlanResult{}, domain.ErrInvalid
	}
	now := s.clock.Now()
	plan := domain.LoadTestPlan{ID: idgen.New("ltp"), ProjectID: input.ProjectID, Name: input.Name, Status: domain.LoadPlanDraft, Version: 1, CreatedAt: now}
	sequences := make(map[int]struct{}, len(input.Cases))
	for index := range input.Cases {
		input.Cases[index].ID = idgen.New("case")
		input.Cases[index].PlanID = plan.ID
		if input.Cases[index].Sequence == 0 {
			input.Cases[index].Sequence = index + 1
		}
		item := input.Cases[index]
		if item.Name == "" || item.Sequence < 1 || item.TargetTonnes <= 0 || item.HoldSeconds <= 0 {
			return CreateLoadPlanResult{}, domain.ErrInvalid
		}
		if _, exists := sequences[item.Sequence]; exists {
			return CreateLoadPlanResult{}, domain.ErrConflict
		}
		sequences[item.Sequence] = struct{}{}
	}
	codes := make(map[string]struct{}, len(input.Channels))
	for index := range input.Channels {
		input.Channels[index].ID = idgen.New("chn")
		input.Channels[index].PlanID = plan.ID
		channel := input.Channels[index]
		if channel.Code == "" || channel.Unit == "" || channel.MinValue > channel.MaxValue {
			return CreateLoadPlanResult{}, domain.ErrInvalid
		}
		if _, exists := codes[channel.Code]; exists {
			return CreateLoadPlanResult{}, domain.ErrConflict
		}
		codes[channel.Code] = struct{}{}
	}
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		if _, err := repo.GetProject(ctx, p.Organization, input.ProjectID); err != nil {
			return err
		}
		return repo.CreateLoadPlan(ctx, plan, input.Cases, input.Channels)
	})
	if err != nil {
		return CreateLoadPlanResult{}, fmt.Errorf("create load plan transaction: %w", err)
	}
	return CreateLoadPlanResult{Plan: plan, Cases: input.Cases, Channels: input.Channels}, nil
}

func (s *Service) ApproveLoadPlan(ctx context.Context, p domain.Principal, projectID, planID string, expected int64) (domain.LoadTestPlan, error) {
	if err := requireRole(p, domain.RoleSupervisor); err != nil {
		return domain.LoadTestPlan{}, err
	}
	var result domain.LoadTestPlan
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		plan, err := repo.GetLoadPlan(ctx, p.Organization, projectID, planID)
		if err != nil {
			return err
		}
		if plan.Version != expected {
			return &domain.VersionConflict{Entity: "load plan", ID: planID, Version: expected}
		}
		cases, channels, err := repo.CountPlanParts(ctx, planID)
		if err != nil {
			return err
		}
		if err := plan.Approve(p.UserID, cases, channels, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.UpdateLoadPlan(ctx, plan, expected); err != nil {
			return err
		}
		result = plan
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("approve load plan: %w", err)
	}
	return result, nil
}

func (s *Service) StartLoadRun(ctx context.Context, p domain.Principal, projectID, planID string) (domain.LoadTestRun, error) {
	if err := requireRole(p, domain.RoleCommissioning); err != nil {
		return domain.LoadTestRun{}, err
	}
	plan, err := s.store.GetLoadPlan(ctx, p.Organization, projectID, planID)
	if err != nil {
		return domain.LoadTestRun{}, err
	}
	if plan.Status != domain.LoadPlanApproved {
		return domain.LoadTestRun{}, &domain.StateError{Entity: "load plan", From: string(plan.Status), To: "run", Reason: "plan approval is required"}
	}
	run := domain.LoadTestRun{ID: idgen.New("run"), PlanID: planID, Status: domain.LoadRunPending, StartedBy: p.UserID, Version: 1}
	if err := run.Transition(domain.LoadRunRunning, s.clock.Now()); err != nil {
		return run, err
	}
	if err := s.store.CreateLoadRun(ctx, run); err != nil {
		return run, fmt.Errorf("create load run: %w", err)
	}
	return run, nil
}

func (s *Service) AppendReading(ctx context.Context, p domain.Principal, runID string, reading domain.SensorReading) error {
	if err := requireRole(p, domain.RoleCommissioning); err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		run, err := repo.GetLoadRun(ctx, p.Organization, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.LoadRunRunning {
			return &domain.StateError{Entity: "load run", From: string(run.Status), To: "append reading", Reason: "run is not collecting"}
		}
		channel, err := repo.GetChannel(ctx, run.PlanID, reading.ChannelID)
		if err != nil {
			return err
		}
		reading.RunID = runID
		if err := reading.Validate(channel); err != nil {
			return err
		}
		return repo.AppendReading(ctx, reading)
	})
}

func (s *Service) QueueLoadEvaluation(ctx context.Context, p domain.Principal, runID string) (domain.LoadTestRun, error) {
	if err := requireRole(p, domain.RoleCommissioning); err != nil {
		return domain.LoadTestRun{}, err
	}
	var result domain.LoadTestRun
	err := s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		run, err := repo.GetLoadRun(ctx, p.Organization, runID)
		if err != nil {
			return err
		}
		expected := run.Version
		if err := run.Transition(domain.LoadRunEvaluating, s.clock.Now()); err != nil {
			return err
		}
		if err := repo.UpdateLoadRun(ctx, run, expected); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"run_id": runID})
		job := domain.Job{ID: idgen.New("job"), Kind: "evaluate_load_run", Payload: payload, Status: domain.JobPending, MaxAttempts: 5, AvailableAt: s.clock.Now(), CreatedAt: s.clock.Now()}
		if err := repo.EnqueueJob(ctx, job); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("queue load evaluation: %w", err)
	}
	return result, nil
}

func (s *Service) EvaluateLoadRun(ctx context.Context, runID string) error {
	return s.store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		run, err := repo.GetLoadRunForEvaluation(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.LoadRunEvaluating {
			return &domain.StateError{Entity: "load run", From: string(run.Status), To: "evaluation result", Reason: "run is not evaluating"}
		}
		passed, failures, err := repo.EvaluateRun(ctx, runID)
		if err != nil {
			return err
		}
		expected := run.Version
		if passed {
			err = run.Transition(domain.LoadRunPassed, s.clock.Now())
		} else {
			err = run.Transition(domain.LoadRunFailed, s.clock.Now())
			run.Failure = fmt.Sprintf("%d mandatory or threshold checks failed", failures)
		}
		if err != nil {
			return err
		}
		return repo.UpdateLoadRun(ctx, run, expected)
	})
}
