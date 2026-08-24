package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLogoutInvalidatesPreviouslyCachedBearerSession(t *testing.T) {
	api := newTestAPI(t)
	first := login(t, api, "owner@example.test", testPassword)
	second := login(t, api, "owner@example.test", testPassword)

	created := request(t, api.handler, http.MethodPost, "/v1/projects", first.Token, map[string]any{
		"name":           "Yangtze approach span",
		"target_open_at": "2026-12-28T00:00:00Z",
		"timezone":       "Asia/Shanghai",
	}, map[string]string{"Idempotency-Key": "logout-cache-project"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", created.Code, created.Body.String())
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil || project.ID == "" {
		t.Fatalf("decode created project: id=%q error=%v body=%s", project.ID, err, created.Body.String())
	}

	projectPath := "/v1/projects/" + project.ID
	for _, session := range []struct {
		name  string
		token string
	}{{"revoked session", first.Token}, {"active session", second.Token}} {
		warm := request(t, api.handler, http.MethodGet, projectPath, session.token, nil, nil)
		if warm.Code != http.StatusOK || !strings.Contains(warm.Body.String(), "Yangtze approach span") {
			t.Fatalf("warm %s cache response = %d/%s", session.name, warm.Code, warm.Body.String())
		}
	}

	logout := request(t, api.handler, http.MethodPost, "/v1/auth/logout", first.Token, nil, nil)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}

	revoked := request(t, api.handler, http.MethodGet, projectPath, first.Token, nil, nil)
	if revoked.Code != http.StatusUnauthorized || errorCode(t, revoked) != "unauthorized" {
		t.Fatalf("cached revoked token response = %d/%s", revoked.Code, revoked.Body.String())
	}
	if strings.Contains(revoked.Body.String(), "Yangtze approach span") {
		t.Fatalf("revoked token leaked project data: %s", revoked.Body.String())
	}

	active := request(t, api.handler, http.MethodGet, projectPath, second.Token, nil, nil)
	if active.Code != http.StatusOK || !strings.Contains(active.Body.String(), "Yangtze approach span") {
		t.Fatalf("unrevoked session response = %d/%s", active.Code, active.Body.String())
	}
}
