package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRejectedInspectionCompletionRollsBackEveryWrite(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	contractor := login(t, api, "contractor@example.test", testPassword)
	supervisor := login(t, api, "supervisor@example.test", testPassword)

	projectResponse := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, map[string]any{
		"name": "Atomic inspection project", "target_open_at": "2026-12-28T00:00:00Z", "timezone": "Asia/Shanghai",
	}, nil)
	if projectResponse.Code != http.StatusCreated {
		t.Fatalf("create project = %d/%s", projectResponse.Code, projectResponse.Body.String())
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(projectResponse.Body.Bytes(), &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	workResponse := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/work-packages", contractor.Token, map[string]any{
		"code": "ATOMIC-INSPECTION", "title": "Atomic inspection", "scope": "Validate rollback", "risk": "medium", "owner_id": "contractor", "due_at": "2026-10-01T00:00:00Z",
	}, nil)
	if workResponse.Code != http.StatusCreated {
		t.Fatalf("create work = %d/%s", workResponse.Code, workResponse.Body.String())
	}
	var work struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(workResponse.Body.Bytes(), &work); err != nil {
		t.Fatalf("decode work: %v", err)
	}
	for _, status := range []string{"active", "submitted"} {
		response := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", contractor.Token, map[string]any{"status": status, "version": work.Version}, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("transition %s = %d/%s", status, response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &work); err != nil {
			t.Fatalf("decode transition: %v", err)
		}
	}
	inspectionResponse := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/inspections", supervisor.Token, map[string]any{
		"inspector_id": "supervisor", "checklist": "Rollback checklist", "scheduled_at": "2026-09-10T02:00:00Z",
	}, nil)
	if inspectionResponse.Code != http.StatusCreated {
		t.Fatalf("schedule inspection = %d/%s", inspectionResponse.Code, inspectionResponse.Body.String())
	}
	var inspection struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(inspectionResponse.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("decode inspection: %v", err)
	}

	failed := request(t, api.handler, http.MethodPost, "/v1/inspections/"+inspection.ID+"/complete", supervisor.Token, map[string]any{
		"passed": false,
		"findings": []map[string]any{
			{"severity": "major", "summary": "Valid first finding", "due_at": "2026-09-15T00:00:00Z"},
			{"severity": "not-a-severity", "summary": "Invalid second finding", "due_at": "2026-09-16T00:00:00Z"},
		},
	}, nil)
	if failed.Code != http.StatusBadRequest {
		t.Fatalf("rejected completion = %d/%s", failed.Code, failed.Body.String())
	}

	stored, err := api.store.GetInspection(t.Context(), "org-test", inspection.ID)
	if err != nil {
		t.Fatalf("read inspection after rejection: %v", err)
	}
	if stored.Status != "scheduled" || stored.Version != 1 {
		t.Fatalf("inspection leaked after rejection: %+v", stored)
	}
	open, err := api.store.CountOpenFindings(t.Context(), inspection.ID)
	if err != nil {
		t.Fatalf("count findings after rejection: %v", err)
	}
	if open != 0 {
		t.Fatalf("findings leaked after rejection: %d", open)
	}
}
