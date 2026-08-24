package httpapi

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

func TestCreateProjectFailureLeavesNoDurableState(t *testing.T) {
	ctx := context.Background()
	clock := repository.FixedClock{Time: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)}
	dbPath := filepath.Join(t.TempDir(), "atomicity.db")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	users := []sqlite.BootstrapUser{
		{ID: "owner", Email: "owner@example.test", DisplayName: "Owner", Role: domain.RoleOwnerAdmin, Password: testPassword},
		{ID: "contractor", Email: "contractor@example.test", DisplayName: "Contractor", Role: domain.RoleContractorEngineer, Password: testPassword},
	}
	if err := store.Bootstrap(ctx, "org-test", "Test Bridge Organization", users, clock.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	raw, err := sql.Open("sqlite", "file:"+url.PathEscape(dbPath)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	if _, err := raw.ExecContext(ctx, `CREATE TRIGGER fail_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'forced audit failure'); END`); err != nil {
		t.Fatalf("create audit trigger: %v", err)
	}
	api := New(service.New(store, clock), 8*time.Hour, 32<<10, slog.New(slog.NewTextHandler(io.Discard, nil)))
	owner := login(t, testAPI{handler: api}, "owner@example.test", testPassword)
	input := map[string]any{
		"name":           "Yanji bridge audit atomicity",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}
	headers := map[string]string{"Idempotency-Key": "atomicity-project-1", "X-Request-ID": "atomicity-request-1"}
	failed := request(t, api, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if failed.Code < http.StatusInternalServerError {
		t.Fatalf("forced audit failure status = %d, body = %s", failed.Code, failed.Body.String())
	}
	listAfterFailure := request(t, api, http.MethodGet, "/v1/projects?limit=10", owner.Token, nil, nil)
	if listAfterFailure.Code != http.StatusOK || !strings.Contains(listAfterFailure.Body.String(), `"total":0`) {
		t.Fatalf("projects after failed create = %d/%s", listAfterFailure.Code, listAfterFailure.Body.String())
	}
	if _, err := raw.ExecContext(ctx, `DROP TRIGGER fail_audit`); err != nil {
		t.Fatalf("drop audit trigger: %v", err)
	}
	retry := request(t, api, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if retry.Code != http.StatusCreated {
		t.Fatalf("retry status = %d, body = %s", retry.Code, retry.Body.String())
	}
	replay := request(t, api, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	listAfterRetry := request(t, api, http.MethodGet, "/v1/projects?limit=10", owner.Token, nil, nil)
	if listAfterRetry.Code != http.StatusOK || !strings.Contains(listAfterRetry.Body.String(), `"total":1`) {
		t.Fatalf("projects after retry = %d/%s", listAfterRetry.Code, listAfterRetry.Body.String())
	}
}
