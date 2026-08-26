package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
)

const testPassword = "BridgeWatch!2026"

type testAPI struct {
	handler http.Handler
	store   *sqlite.DB
	service *service.Service
	clock   repository.FixedClock
}

func newTestAPI(t *testing.T) testAPI {
	t.Helper()
	clock := repository.FixedClock{Time: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)}
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	users := []sqlite.BootstrapUser{
		{ID: "owner", Email: "owner@example.test", DisplayName: "Owner", Role: domain.RoleOwnerAdmin, Password: testPassword},
		{ID: "contractor", Email: "contractor@example.test", DisplayName: "Contractor", Role: domain.RoleContractorEngineer, Password: testPassword},
		{ID: "supervisor", Email: "supervisor@example.test", DisplayName: "Supervisor", Role: domain.RoleSupervisor, Password: testPassword},
		{ID: "commissioning", Email: "commissioning@example.test", DisplayName: "Commissioning", Role: domain.RoleCommissioning, Password: testPassword},
	}
	if err := store.Bootstrap(context.Background(), "org-test", "Test Bridge Organization", users, clock.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(store, clock)
	return testAPI{
		handler: New(svc, 8*time.Hour, 32<<10, logger),
		store:   store,
		service: svc,
		clock:   clock,
	}
}

func request(t *testing.T, handler http.Handler, method, path, token string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func login(t *testing.T, api testAPI, email, password string) LoginResultForTest {
	t.Helper()
	response := request(t, api.handler, http.MethodPost, "/v1/auth/login", "", map[string]string{
		"email":    email,
		"password": password,
	}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", response.Code, response.Body.String())
	}
	var result LoginResultForTest
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if result.Token == "" || result.User.UserID == "" || result.ExpiresAt.IsZero() {
		t.Fatalf("incomplete login response: %+v", result)
	}
	return result
}

type LoginResultForTest struct {
	Token     string           `json:"token"`
	ExpiresAt time.Time        `json:"expires_at"`
	User      domain.Principal `json:"user"`
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response %q: %v", response.Body.String(), err)
	}
	if payload.Error.RequestID == "" {
		t.Fatalf("error response has no request_id: %s", response.Body.String())
	}
	return payload.Error.Code
}

func TestHealthAndReadinessArePublic(t *testing.T) {
	api := newTestAPI(t)
	tests := []struct {
		path string
		want string
	}{
		{path: "/healthz", want: `"status":"alive"`},
		{path: "/readyz", want: `"status":"ready"`},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := request(t, api.handler, http.MethodGet, test.path, "", nil, map[string]string{"X-Request-ID": "probe-123"})
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("body = %s, want fragment %s", response.Body.String(), test.want)
			}
			if got := response.Header().Get("X-Request-ID"); got != "probe-123" {
				t.Fatalf("request id = %q, want probe-123", got)
			}
		})
	}
}

func TestProtectedEndpointRejectsMissingAndMalformedAuthentication(t *testing.T) {
	api := newTestAPI(t)
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing", header: ""},
		{name: "wrong scheme", header: "Basic abc"},
		{name: "missing token", header: "Bearer "},
		{name: "unknown token", header: "Bearer not-a-real-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
			if test.header != "" {
				req.Header.Set("Authorization", test.header)
			}
			recorder := httptest.NewRecorder()
			api.handler.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if got := errorCode(t, recorder); got != "unauthorized" {
				t.Fatalf("error code = %q, want unauthorized", got)
			}
		})
	}
}

func TestLoginRejectsCredentialsAndUnknownFields(t *testing.T) {
	api := newTestAPI(t)
	tests := []struct {
		name       string
		body       any
		wantStatus int
		wantCode   string
	}{
		{name: "wrong password", body: map[string]string{"email": "owner@example.test", "password": "wrong-password"}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "unknown email", body: map[string]string{"email": "missing@example.test", "password": testPassword}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "missing password", body: map[string]string{"email": "owner@example.test"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unknown field", body: map[string]string{"email": "owner@example.test", "password": testPassword, "role": "owner_admin"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, api.handler, http.MethodPost, "/v1/auth/login", "", test.body, nil)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if got := errorCode(t, response); got != test.wantCode {
				t.Fatalf("code = %q, want %q", got, test.wantCode)
			}
		})
	}
}

func TestOwnerCreatesListsAndReadsProjectWithIdempotency(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	input := map[string]any{
		"name":           "Yanji year-end opening readiness",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}
	headers := map[string]string{
		"Idempotency-Key": "create-yanji-project",
		"X-Request-ID":    "project-create-1",
	}
	first := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", first.Code, first.Body.String())
	}
	var created domain.Project
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if created.ID == "" || created.Organization != "org-test" || created.Status != domain.ProjectCloseout || created.Version != 1 {
		t.Fatalf("created project = %+v", created)
	}
	second := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, input, headers)
	if second.Code != http.StatusCreated {
		t.Fatalf("idempotent create status = %d, body = %s", second.Code, second.Body.String())
	}
	var replayed domain.Project
	if err := json.Unmarshal(second.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if replayed.ID != created.ID || !replayed.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("replayed project = %+v, original = %+v", replayed, created)
	}
	conflictingInput := map[string]any{
		"name":           "Different project",
		"target_open_at": "2026-12-29T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}
	conflict := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, conflictingInput, headers)
	if conflict.Code != http.StatusConflict || errorCode(t, conflict) != "conflict" {
		t.Fatalf("idempotency conflict status/body = %d/%s", conflict.Code, conflict.Body.String())
	}
	list := request(t, api.handler, http.MethodGet, "/v1/projects?limit=10&sort=target_open_at", owner.Token, nil, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), created.ID) || !strings.Contains(list.Body.String(), `"total":1`) {
		t.Fatalf("list response = %d/%s", list.Code, list.Body.String())
	}
	get := request(t, api.handler, http.MethodGet, "/v1/projects/"+created.ID, owner.Token, nil, nil)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), created.Name) {
		t.Fatalf("get response = %d/%s", get.Code, get.Body.String())
	}
}

func TestRoleAuthorizationAndLogoutRevocation(t *testing.T) {
	api := newTestAPI(t)
	contractor := login(t, api, "contractor@example.test", testPassword)
	create := request(t, api.handler, http.MethodPost, "/v1/projects", contractor.Token, map[string]any{
		"name":           "Unauthorized project",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}, nil)
	if create.Code != http.StatusForbidden || errorCode(t, create) != "forbidden" {
		t.Fatalf("contractor create response = %d/%s", create.Code, create.Body.String())
	}
	list := request(t, api.handler, http.MethodGet, "/v1/projects", contractor.Token, nil, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("contractor list status = %d, body = %s", list.Code, list.Body.String())
	}
	logout := request(t, api.handler, http.MethodPost, "/v1/auth/logout", contractor.Token, nil, nil)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	afterLogout := request(t, api.handler, http.MethodGet, "/v1/projects", contractor.Token, nil, nil)
	if afterLogout.Code != http.StatusUnauthorized || errorCode(t, afterLogout) != "unauthorized" {
		t.Fatalf("revoked token response = %d/%s", afterLogout.Code, afterLogout.Body.String())
	}
}

func TestRequestBodySizeAndRequestIDValidation(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	oversized := strings.Repeat("x", 40<<10)
	response := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, map[string]string{
		"name":           oversized,
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}, map[string]string{"X-Request-ID": "contains a space"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d, body = %s", response.Code, response.Body.String())
	}
	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" || requestID == "contains a space" {
		t.Fatalf("unsafe request ID was not replaced: %q", requestID)
	}
}

func TestCloseoutLoadTestAndHandoverWorkflow(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	contractor := login(t, api, "contractor@example.test", testPassword)
	supervisor := login(t, api, "supervisor@example.test", testPassword)
	commissioning := login(t, api, "commissioning@example.test", testPassword)

	projectResponse := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, map[string]any{
		"name": "Yanji bridge commissioning", "target_open_at": "2026-12-28T00:00:00Z", "timezone": "Asia/Shanghai",
	}, nil)
	if projectResponse.Code != http.StatusCreated {
		t.Fatalf("create project = %d/%s", projectResponse.Code, projectResponse.Body.String())
	}
	var project domain.Project
	if err := json.Unmarshal(projectResponse.Body.Bytes(), &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	workResponse := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/work-packages", contractor.Token, map[string]any{
		"code": "TOWER-COAT", "title": "Tower coating closeout", "scope": "Complete coating records", "risk": "high", "owner_id": "contractor", "due_at": "2026-10-01T00:00:00Z",
	}, nil)
	if workResponse.Code != http.StatusCreated {
		t.Fatalf("create work = %d/%s", workResponse.Code, workResponse.Body.String())
	}
	var work domain.WorkPackage
	if err := json.Unmarshal(workResponse.Body.Bytes(), &work); err != nil {
		t.Fatalf("decode work: %v", err)
	}
	for _, status := range []domain.WorkStatus{domain.WorkActive, domain.WorkSubmitted} {
		response := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", contractor.Token, map[string]any{"status": status, "version": work.Version}, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("transition to %s = %d/%s", status, response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &work); err != nil {
			t.Fatalf("decode work transition: %v", err)
		}
	}
	earlyAcceptance := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", supervisor.Token, map[string]any{"status": domain.WorkAccepted, "version": work.Version}, nil)
	if earlyAcceptance.Code != http.StatusConflict || errorCode(t, earlyAcceptance) != "conflict" {
		t.Fatalf("accept without inspection = %d/%s", earlyAcceptance.Code, earlyAcceptance.Body.String())
	}

	inspectionResponse := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/inspections", supervisor.Token, map[string]any{
		"inspector_id": "supervisor", "checklist": "Coating thickness and adhesion", "scheduled_at": "2026-09-10T02:00:00Z",
	}, nil)
	if inspectionResponse.Code != http.StatusCreated {
		t.Fatalf("schedule inspection = %d/%s", inspectionResponse.Code, inspectionResponse.Body.String())
	}
	var inspection domain.Inspection
	if err := json.Unmarshal(inspectionResponse.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("decode inspection: %v", err)
	}
	completeResponse := request(t, api.handler, http.MethodPost, "/v1/inspections/"+inspection.ID+"/complete", supervisor.Token, map[string]any{"passed": false, "findings": []map[string]any{{"severity": "major", "summary": "Coating adhesion below acceptance threshold", "due_at": "2026-09-15T00:00:00Z"}}}, nil)
	if completeResponse.Code != http.StatusOK {
		t.Fatalf("complete inspection = %d/%s", completeResponse.Code, completeResponse.Body.String())
	}
	var failedInspection service.CompleteInspectionResult
	if err := json.Unmarshal(completeResponse.Body.Bytes(), &failedInspection); err != nil || len(failedInspection.Findings) != 1 {
		t.Fatalf("decode failed inspection = %+v/%v", failedInspection, err)
	}
	finding := failedInspection.Findings[0]
	resolve := request(t, api.handler, http.MethodPost, "/v1/findings/"+finding.ID+"/resolve", contractor.Token, map[string]any{"resolution": "Recoated and verified adhesion", "version": finding.Version}, nil)
	if resolve.Code != http.StatusOK {
		t.Fatalf("resolve finding = %d/%s", resolve.Code, resolve.Body.String())
	}
	reinspectionResponse := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/inspections", supervisor.Token, map[string]any{
		"inspector_id": "supervisor", "checklist": "Coating adhesion reinspection", "scheduled_at": "2026-09-16T02:00:00Z",
	}, nil)
	if reinspectionResponse.Code != http.StatusCreated {
		t.Fatalf("schedule reinspection = %d/%s", reinspectionResponse.Code, reinspectionResponse.Body.String())
	}
	if err := json.Unmarshal(reinspectionResponse.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("decode reinspection: %v", err)
	}
	reinspectionComplete := request(t, api.handler, http.MethodPost, "/v1/inspections/"+inspection.ID+"/complete", supervisor.Token, map[string]any{"passed": true, "findings": []any{}}, nil)
	if reinspectionComplete.Code != http.StatusOK {
		t.Fatalf("complete reinspection = %d/%s", reinspectionComplete.Code, reinspectionComplete.Body.String())
	}
	acceptResponse := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", supervisor.Token, map[string]any{"status": domain.WorkAccepted, "version": work.Version}, nil)
	if acceptResponse.Code != http.StatusOK {
		t.Fatalf("accept inspected work = %d/%s", acceptResponse.Code, acceptResponse.Body.String())
	}

	planResponse := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/load-plans", commissioning.Token, map[string]any{
		"name": "Static load commissioning", "cases": []map[string]any{{"name": "Mid-span design load", "target_tonnes": 720, "hold_seconds": 300}}, "channels": []map[string]any{{"code": "DEFLECT-MID", "unit": "mm", "min_value": -8, "max_value": 8, "mandatory": true}},
	}, nil)
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("create load plan = %d/%s", planResponse.Code, planResponse.Body.String())
	}
	var planResult service.CreateLoadPlanResult
	if err := json.Unmarshal(planResponse.Body.Bytes(), &planResult); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	plan := planResult.Plan
	approvePlan := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/load-plans/"+plan.ID+"/approve", supervisor.Token, map[string]any{"version": plan.Version}, nil)
	if approvePlan.Code != http.StatusOK {
		t.Fatalf("approve load plan = %d/%s", approvePlan.Code, approvePlan.Body.String())
	}
	startRun := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/load-plans/"+plan.ID+"/runs", commissioning.Token, nil, nil)
	if startRun.Code != http.StatusCreated {
		t.Fatalf("start run = %d/%s", startRun.Code, startRun.Body.String())
	}
	var run domain.LoadTestRun
	if err := json.Unmarshal(startRun.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if len(planResult.Channels) != 1 || planResult.Channels[0].ID == "" {
		t.Fatalf("created plan channels = %+v", planResult.Channels)
	}
	channelID := planResult.Channels[0].ID
	reading := request(t, api.handler, http.MethodPost, "/v1/load-runs/"+run.ID+"/readings", commissioning.Token, map[string]any{"channel_id": channelID, "sequence": 1, "value": 2.4, "observed_at": "2026-09-20T04:00:00Z"}, nil)
	if reading.Code != http.StatusNoContent {
		t.Fatalf("append reading = %d/%s", reading.Code, reading.Body.String())
	}
	queued := request(t, api.handler, http.MethodPost, "/v1/load-runs/"+run.ID+"/evaluate", commissioning.Token, nil, nil)
	if queued.Code != http.StatusAccepted {
		t.Fatalf("queue evaluation = %d/%s", queued.Code, queued.Body.String())
	}
	if err := api.service.EvaluateLoadRun(context.Background(), run.ID); err != nil {
		t.Fatalf("EvaluateLoadRun() error = %v", err)
	}

	dossierResponse := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/dossiers", owner.Token, map[string]any{"required_documents": []string{"completion-certificate", "load-test-report"}}, nil)
	if dossierResponse.Code != http.StatusCreated {
		t.Fatalf("create dossier = %d/%s", dossierResponse.Code, dossierResponse.Body.String())
	}
	var dossier domain.HandoverDossier
	if err := json.Unmarshal(dossierResponse.Body.Bytes(), &dossier); err != nil {
		t.Fatalf("decode dossier: %v", err)
	}
	for _, kind := range []string{"completion-certificate", "load-test-report"} {
		document := request(t, api.handler, http.MethodPut, "/v1/projects/"+project.ID+"/dossiers/"+dossier.ID+"/documents/"+kind, owner.Token, map[string]string{"uri": "bridgewatch://documents/" + kind}, nil)
		if document.Code != http.StatusNoContent {
			t.Fatalf("receive %s = %d/%s", kind, document.Code, document.Body.String())
		}
	}
	submit := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/dossiers/"+dossier.ID+"/submit", owner.Token, map[string]any{"version": dossier.Version}, nil)
	if submit.Code != http.StatusOK {
		t.Fatalf("submit dossier = %d/%s", submit.Code, submit.Body.String())
	}
	if err := json.Unmarshal(submit.Body.Bytes(), &dossier); err != nil {
		t.Fatalf("decode submitted dossier: %v", err)
	}
	decision := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/dossiers/"+dossier.ID+"/decision", supervisor.Token, map[string]any{"approve": true, "note": "All closeout evidence verified", "version": dossier.Version}, nil)
	if decision.Code != http.StatusOK {
		t.Fatalf("approve dossier = %d/%s", decision.Code, decision.Body.String())
	}
	readiness := request(t, api.handler, http.MethodGet, "/v1/projects/"+project.ID+"/opening-readiness", owner.Token, nil, nil)
	if readiness.Code != http.StatusOK || !strings.Contains(readiness.Body.String(), `"ready":true`) || !strings.Contains(readiness.Body.String(), `"blockers":[]`) {
		t.Fatalf("opening readiness = %d/%s", readiness.Code, readiness.Body.String())
	}
}

func TestOrganizationBoundaryHidesLoadAndDossierResources(t *testing.T) {
	api := newTestAPI(t)
	now := api.clock.Now()
	if err := api.store.Bootstrap(context.Background(), "org-other", "Other Organization", []sqlite.BootstrapUser{
		{ID: "other-owner", Email: "other-owner@example.test", DisplayName: "Other Owner", Role: domain.RoleOwnerAdmin, Password: testPassword},
		{ID: "other-commissioning", Email: "other-commissioning@example.test", DisplayName: "Other Commissioning", Role: domain.RoleCommissioning, Password: testPassword},
	}, now); err != nil {
		t.Fatalf("bootstrap other organization: %v", err)
	}
	owner := domain.Principal{UserID: "owner", Organization: "org-test", Role: domain.RoleOwnerAdmin}
	other := domain.Principal{UserID: "other-owner", Organization: "org-other", Role: domain.RoleOwnerAdmin}
	project, err := api.service.CreateProject(context.Background(), owner, "tenant-project", "", service.CreateProjectInput{Name: "Tenant protected project", TargetOpenAt: now.Add(90 * 24 * time.Hour), Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	dossier, err := api.service.CreateDossier(context.Background(), owner, project.ID, []string{"handover-record"})
	if err != nil {
		t.Fatalf("CreateDossier() error = %v", err)
	}
	err = api.service.ReceiveDossierDocument(context.Background(), other, project.ID, dossier.ID, "handover-record", "bridgewatch://foreign")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign dossier write error = %v, want not found", err)
	}
}
