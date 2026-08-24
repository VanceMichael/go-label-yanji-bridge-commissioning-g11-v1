package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/middleware"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	result, err := s.service.Login(r.Context(), input.Email, input.Password, s.sessionTTL)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.Principal(r.Context())
	if !ok {
		s.writeError(w, r, domain.ErrUnauthorized)
		return
	}
	if err := s.service.Logout(r.Context(), principal); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
