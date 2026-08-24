package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestStaleWorkVersionPreservesConflictResponseContract(t *testing.T) {
	api := newTestAPI(t)
	owner := login(t, api, "owner@example.test", testPassword)
	contractor := login(t, api, "contractor@example.test", testPassword)

	projectResponse := request(t, api.handler, http.MethodPost, "/v1/projects", owner.Token, map[string]any{
		"name":           "Stale version bridge project",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}, nil)
	if projectResponse.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", projectResponse.Code, projectResponse.Body.String())
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(projectResponse.Body.Bytes(), &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	workResponse := request(t, api.handler, http.MethodPost, "/v1/projects/"+project.ID+"/work-packages", contractor.Token, map[string]any{
		"code":     "STALE-VERSION",
		"title":    "Versioned transition",
		"scope":    "Preserve conflict contract",
		"risk":     "medium",
		"owner_id": "contractor",
		"due_at":   "2026-10-01T00:00:00Z",
	}, nil)
	if workResponse.Code != http.StatusCreated {
		t.Fatalf("create work status = %d, body = %s", workResponse.Code, workResponse.Body.String())
	}
	var work struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(workResponse.Body.Bytes(), &work); err != nil {
		t.Fatalf("decode work: %v", err)
	}

	advanced := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", contractor.Token, map[string]any{
		"status":  "active",
		"version": work.Version,
	}, nil)
	if advanced.Code != http.StatusOK {
		t.Fatalf("advance work status = %d, body = %s", advanced.Code, advanced.Body.String())
	}
	work.Version++

	stale := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", contractor.Token, map[string]any{
		"status":  "submitted",
		"version": work.Version - 1,
	}, map[string]string{"X-Request-ID": "stale-work-version"})
	if stale.Code != http.StatusConflict || errorCode(t, stale) != "conflict" {
		t.Fatalf("stale transition response = %d/%s, want HTTP 409 conflict", stale.Code, stale.Body.String())
	}

	retry := request(t, api.handler, http.MethodPost, "/v1/work-packages/"+work.ID+"/transitions", contractor.Token, map[string]any{
		"status":  "submitted",
		"version": work.Version,
	}, nil)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry transition status = %d, body = %s", retry.Code, retry.Body.String())
	}
}
