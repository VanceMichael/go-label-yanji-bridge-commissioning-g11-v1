package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

func newWorkerStore(t *testing.T) *sqlite.DB {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	return store
}

func newTestRunner(store repository.Store, now time.Time) *Runner {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := New(store, "worker-test", time.Millisecond, logger)
	runner.clock = repository.FixedClock{Time: now}
	runner.lease = time.Minute
	return runner
}

func enqueue(t *testing.T, store repository.Store, job domain.Job) {
	t.Helper()
	if err := store.EnqueueJob(context.Background(), job); err != nil {
		t.Fatalf("EnqueueJob() error = %v", err)
	}
}

func TestRunnerOnceCompletesSuccessfulJob(t *testing.T) {
	store := newWorkerStore(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runner := newTestRunner(store, now)
	var calls atomic.Int64
	runner.Register("success", func(ctx context.Context, job domain.Job) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("handler context error = %v", err)
		}
		if job.ID != "job-success" || string(job.Payload) != `{"bridge":"yanji"}` {
			t.Fatalf("handler job = %+v", job)
		}
		calls.Add(1)
		return nil
	})
	enqueue(t, store, domain.Job{
		ID:          "job-success",
		Kind:        "success",
		Payload:     []byte(`{"bridge":"yanji"}`),
		Status:      domain.JobPending,
		MaxAttempts: 3,
		AvailableAt: now,
		CreatedAt:   now,
	})
	if err := runner.once(context.Background()); err != nil {
		t.Fatalf("once() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	if _, err := store.ClaimJob(context.Background(), "other-worker", now.Add(10*time.Minute), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("completed job was claimable: %v", err)
	}
}

func TestRunnerRetriesThenPersistsPermanentFailure(t *testing.T) {
	store := newWorkerStore(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runner := newTestRunner(store, now)
	var calls atomic.Int64
	runner.Register("always-fails", func(context.Context, domain.Job) error {
		calls.Add(1)
		return errors.New("sensor quorum unavailable")
	})
	enqueue(t, store, domain.Job{
		ID:          "job-failure",
		Kind:        "always-fails",
		Payload:     []byte(`{}`),
		Status:      domain.JobPending,
		MaxAttempts: 2,
		AvailableAt: now,
		CreatedAt:   now,
	})
	firstErr := runner.once(context.Background())
	if firstErr == nil || firstErr.Error() != "sensor quorum unavailable" {
		t.Fatalf("first once() error = %v", firstErr)
	}
	runner.clock = repository.FixedClock{Time: now.Add(time.Second)}
	secondErr := runner.once(context.Background())
	if secondErr == nil || secondErr.Error() != "sensor quorum unavailable" {
		t.Fatalf("second once() error = %v", secondErr)
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}
	if _, err := store.ClaimJob(context.Background(), "other-worker", now.Add(time.Hour), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("permanently failed job was claimable: %v", err)
	}
}

func TestRunnerUnknownKindConsumesAttemptAndReturnsEvidence(t *testing.T) {
	store := newWorkerStore(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runner := newTestRunner(store, now)
	enqueue(t, store, domain.Job{
		ID:          "job-unknown",
		Kind:        "missing-handler",
		Payload:     []byte(`{}`),
		Status:      domain.JobPending,
		MaxAttempts: 1,
		AvailableAt: now,
		CreatedAt:   now,
	})
	err := runner.once(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported job kind") {
		t.Fatalf("once() error = %v, want unsupported kind", err)
	}
	if _, err := store.ClaimJob(context.Background(), "other-worker", now.Add(time.Hour), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown permanent job was claimable: %v", err)
	}
}

func TestRunnerCancellationReleasesOwnedLease(t *testing.T) {
	store := newWorkerStore(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	enqueue(t, store, domain.Job{
		ID:          "job-leased",
		Kind:        "later",
		Payload:     []byte(`{}`),
		Status:      domain.JobPending,
		MaxAttempts: 3,
		AvailableAt: now,
		CreatedAt:   now,
	})
	claimed, err := store.ClaimJob(context.Background(), "worker-test", now, time.Hour)
	if err != nil {
		t.Fatalf("initial ClaimJob() error = %v", err)
	}
	if claimed.LeaseOwner != "worker-test" {
		t.Fatalf("lease owner = %q", claimed.LeaseOwner)
	}
	runner := newTestRunner(store, now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = runner.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want canceled", err)
	}
	recovered, err := store.ClaimJob(context.Background(), "replacement-worker", now, time.Minute)
	if err != nil {
		t.Fatalf("claim released job error = %v", err)
	}
	if recovered.ID != "job-leased" || recovered.LeaseOwner != "replacement-worker" {
		t.Fatalf("recovered job = %+v", recovered)
	}
}

func TestRunnerEvaluateFailureRetainsErrorAndAllowsRetry(t *testing.T) {
	store := newWorkerStore(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runner := newTestRunner(store, now)
	runner.interval = time.Second
	var calls atomic.Int64
	runner.Register("evaluate_load_run", func(context.Context, domain.Job) error {
		calls.Add(1)
		return errors.New("sensor quorum unavailable")
	})
	enqueue(t, store, domain.Job{
		ID:          "job-evaluate",
		Kind:        "evaluate_load_run",
		Payload:     []byte(`{"run_id":"run-1"}`),
		Status:      domain.JobPending,
		MaxAttempts: 3,
		AvailableAt: now,
		CreatedAt:   now,
	})
	if err := runner.once(context.Background()); err == nil || err.Error() != "sensor quorum unavailable" {
		t.Fatalf("first once() error = %v, want sensor quorum unavailable", err)
	}
	if _, err := store.ClaimJob(context.Background(), "other-worker", now, time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed evaluation job claimed during backoff: %v", err)
	}
	recovered, err := store.ClaimJob(context.Background(), "other-worker", now.Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("retry ClaimJob() error = %v", err)
	}
	if recovered.ID != "job-evaluate" || recovered.Attempts != 2 {
		t.Fatalf("retried job = %+v, want job-evaluate attempts=2", recovered)
	}
	if recovered.LastError == "" {
		t.Fatalf("retried job retained no error: %+v", recovered)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestRunnerRegisterRejectsInvalidOrDuplicateHandlers(t *testing.T) {
	store := newWorkerStore(t)
	runner := newTestRunner(store, time.Now())
	assertPanic := func(name string, operation func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Fatal("operation did not panic")
				}
			}()
			operation()
		})
	}
	assertPanic("empty kind", func() { runner.Register("", func(context.Context, domain.Job) error { return nil }) })
	assertPanic("nil handler", func() { runner.Register("nil", nil) })
	runner.Register("duplicate", func(context.Context, domain.Job) error { return nil })
	assertPanic("duplicate kind", func() { runner.Register("duplicate", func(context.Context, domain.Job) error { return nil }) })
}
