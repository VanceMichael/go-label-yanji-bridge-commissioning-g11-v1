package httpapi

import (
	"net/http"
	"strconv"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

func principal(r *http.Request) (domain.Principal, error) {
	p, ok := middleware.Principal(r.Context())
	if !ok {
		return p, domain.ErrUnauthorized
	}
	return p, nil
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input service.CreateProjectInput
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	project, err := s.service.CreateProject(r.Context(), p, middleware.RequestID(r.Context()), r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}
func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	project, err := s.service.GetProject(r.Context(), p, r.PathValue("projectID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := s.service.ListProjects(r.Context(), p, repository.Page{Limit: limit, Offset: offset, Sort: r.URL.Query().Get("sort"), Status: r.URL.Query().Get("status")})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}
