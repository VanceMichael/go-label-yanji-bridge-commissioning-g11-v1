package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

type Handler func(context.Context, domain.Job) error

type Runner struct {
	store    repository.Store
	owner    string
	interval time.Duration
	lease    time.Duration
	clock    repository.Clock
	logger   *slog.Logger
	handlers map[string]Handler
}

func New(store repository.Store, owner string, interval time.Duration, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{store: store, owner: owner, interval: interval, lease: 30 * time.Second, clock: repository.RealClock{}, logger: logger, handlers: map[string]Handler{}}
}

func (r *Runner) Register(kind string, handler Handler) {
	if kind == "" || handler == nil {
		panic("worker handler requires kind and function")
	}
	if _, exists := r.handlers[kind]; exists {
		panic("duplicate worker handler: " + kind)
	}
	r.handlers[kind] = handler
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer r.store.ReleaseLeases(context.WithoutCancel(ctx), r.owner, r.clock.Now())
	for {
		if err := r.once(ctx); err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, context.Canceled) {
			r.logger.Error("worker cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) once(ctx context.Context) error {
	job, err := r.store.ClaimJob(ctx, r.owner, r.clock.Now(), r.lease)
	if err != nil {
		return err
	}
	handler, ok := r.handlers[job.Kind]
	if !ok {
		return r.fail(ctx, job, fmt.Errorf("unsupported job kind %q", job.Kind))
	}
	jobCtx, cancel := context.WithTimeout(ctx, r.lease)
	defer cancel()
	acknowledged := false
	if job.Kind == "evaluate_load_run" {
		if err := r.store.CompleteJob(ctx, job.ID, r.owner, r.clock.Now()); err != nil {
			return fmt.Errorf("complete worker job: %w", err)
		}
		acknowledged = true
	}
	if err := handler(jobCtx, job); err != nil {
		return r.fail(ctx, job, err)
	}
	if !acknowledged {
		if err := r.store.CompleteJob(ctx, job.ID, r.owner, r.clock.Now()); err != nil {
			return fmt.Errorf("complete worker job: %w", err)
		}
	}
	return nil
}

func (r *Runner) fail(ctx context.Context, job domain.Job, cause error) error {
	backoff := time.Duration(math.Pow(2, float64(job.Attempts-1))) * r.interval
	if backoff > time.Minute {
		backoff = time.Minute
	}
	if err := r.store.RetryJob(ctx, job, cause.Error(), r.clock.Now().Add(backoff)); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
