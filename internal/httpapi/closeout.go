package httpapi

import (
	"net/http"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

func (s *Server) createWork(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input service.CreateWorkInput
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	input.ProjectID = r.PathValue("projectID")
	work, err := s.service.CreateWorkPackage(r.Context(), p, middleware.RequestID(r.Context()), input)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, work)
}

func (s *Server) scheduleInspection(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		InspectorID string    `json:"inspector_id"`
		Checklist   string    `json:"checklist"`
		ScheduledAt time.Time `json:"scheduled_at"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	inspection, err := s.service.ScheduleInspection(r.Context(), p, r.PathValue("workID"), input.InspectorID, input.Checklist, input.ScheduledAt)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, inspection)
}

func (s *Server) completeInspection(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		Passed   bool             `json:"passed"`
		Findings []domain.Finding `json:"findings"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	result, err := s.service.CompleteInspection(r.Context(), p, r.PathValue("inspectionID"), input.Passed, input.Findings)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) resolveFinding(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		Resolution string `json:"resolution"`
		Version    int64  `json:"version"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	finding, err := s.service.ResolveFinding(r.Context(), p, r.PathValue("findingID"), input.Resolution, input.Version)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, finding)
}
func (s *Server) transitionWork(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		Status  domain.WorkStatus `json:"status"`
		Version int64             `json:"version"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	work, err := s.service.TransitionWork(r.Context(), p, middleware.RequestID(r.Context()), r.PathValue("workID"), input.Status, input.Version)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, work)
}
