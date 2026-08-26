package httpapi

import "net/http"

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	result, err := s.service.OpeningReadiness(r.Context(), p, r.PathValue("projectID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
