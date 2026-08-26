package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

func RegisterBridgeHandlers(runner *Runner, svc *service.Service) {
	runner.Register("evaluate_load_run", func(ctx context.Context, job domain.Job) error {
		var payload struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("decode evaluation job: %w", err)
		}
		if payload.RunID == "" {
			return fmt.Errorf("evaluation job missing run_id: %w", domain.ErrInvalid)
		}
		return svc.EvaluateLoadRun(ctx, payload.RunID)
	})
}
