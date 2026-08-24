package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

type projectAuditFailureStore struct {
	repository.Store
	failNext atomic.Bool
}

type projectAuditFailureRepo struct {
	repository.MutationRepository
}

func (r projectAuditFailureRepo) AppendAudit(ctx context.Context, event domain.AuditEvent) error {
	return r.MutationRepository.AppendAudit(ctx, event)
}

func (s projectAuditFailureStore) WithinTx(ctx context.Context, fn func(repository.MutationRepository) error) error {
	return s.Store.WithinTx(ctx, func(repo repository.MutationRepository) error {
		wrapped := projectAuditFailureRepo{MutationRepository: repo}
		if s.failNext.Swap(false) {
			wrapped.MutationRepository = auditFailingRepo{MutationRepository: repo}
		}
		return fn(wrapped)
	})
}

type auditFailingRepo struct{ repository.MutationRepository }

func (auditFailingRepo) AppendAudit(context.Context, domain.AuditEvent) error {
	return errors.New("injected audit persistence failure")
}

func TestCreateProjectFailureLeavesNoDurableState(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	failingStore := projectAuditFailureStore{Store: api.store}
	failingStore.failNext.Store(true)
	failingHandler := New(
		service.New(failingStore, api.clock),
		8*time.Hour,
		32<<10,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	input := map[string]any{
		"name":           "Atomicity probe project",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}
	headers := map[string]string{
		"Idempotency-Key": "atomicity-probe-0001",
		"X-Request-ID":    "atomicity-probe-request",
	}
	failed := request(t, failingHandler, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if failed.Code != http.StatusInternalServerError || errorCode(t, failed) != "internal_error" {
		t.Fatalf("failed create response = %d/%s", failed.Code, failed.Body.String())
	}
	listAfterFailure := request(t, api.handler, http.MethodGet, "/v1/projects?limit=10", owner.Token, nil, nil)
	var leaked struct {
		Items []domain.Project `json:"items"`
		Total int              `json:"total"`
	}
	if listAfterFailure.Code != http.StatusOK || json.Unmarshal(listAfterFailure.Body.Bytes(), &leaked) != nil {
		t.Fatalf("list after failed create = %d/%s", listAfterFailure.Code, listAfterFailure.Body.String())
	}
	if leaked.Total != 0 || len(leaked.Items) != 0 {
		t.Fatalf("failed create left projects = total %d items %d, want no durable project", leaked.Total, len(leaked.Items))
	}
	retry := request(t, failingHandler, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if retry.Code != http.StatusCreated {
		t.Fatalf("retry status = %d, body = %s", retry.Code, retry.Body.String())
	}
	listAfterRetry := request(t, api.handler, http.MethodGet, "/v1/projects?limit=10", owner.Token, nil, nil)
	var duplicated struct {
		Items []domain.Project `json:"items"`
		Total int              `json:"total"`
	}
	if listAfterRetry.Code != http.StatusOK || json.Unmarshal(listAfterRetry.Body.Bytes(), &duplicated) != nil {
		t.Fatalf("list after retry = %d/%s", listAfterRetry.Code, listAfterRetry.Body.String())
	}
	if duplicated.Total != 1 || len(duplicated.Items) != 1 {
		t.Fatalf("retry created unexpected project set = %+v, want one project", duplicated.Items)
	}
}
