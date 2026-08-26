package httpapi

import (
	"net/http"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

func (s *Server) createLoadPlan(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input service.CreateLoadPlanInput
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	input.ProjectID = r.PathValue("projectID")
	result, err := s.service.CreateLoadPlan(r.Context(), p, input)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) approveLoadPlan(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	plan, err := s.service.ApproveLoadPlan(r.Context(), p, r.PathValue("projectID"), r.PathValue("planID"), input.Version)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
func (s *Server) startLoadRun(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	run, err := s.service.StartLoadRun(r.Context(), p, r.PathValue("projectID"), r.PathValue("planID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}
func (s *Server) appendReading(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var reading domain.SensorReading
	if err := s.decode(w, r, &reading); err != nil {
		s.writeError(w, r, err)
		return
	}
	if reading.ObservedAt.IsZero() {
		reading.ObservedAt = time.Now().UTC()
	}
	if err := s.service.AppendReading(r.Context(), p, r.PathValue("runID"), reading); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) queueEvaluation(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	run, err := s.service.QueueLoadEvaluation(r.Context(), p, r.PathValue("runID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}
