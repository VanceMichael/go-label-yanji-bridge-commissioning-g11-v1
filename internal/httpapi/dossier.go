package httpapi

import "net/http"

func (s *Server) createDossier(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		RequiredDocuments []string `json:"required_documents"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	dossier, err := s.service.CreateDossier(r.Context(), p, r.PathValue("projectID"), input.RequiredDocuments)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dossier)
}

func (s *Server) receiveDossierDocument(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		URI string `json:"uri"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.service.ReceiveDossierDocument(r.Context(), p, r.PathValue("projectID"), r.PathValue("dossierID"), r.PathValue("kind"), input.URI); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) submitDossier(w http.ResponseWriter, r *http.Request) {
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
	dossier, err := s.service.SubmitDossier(r.Context(), p, r.PathValue("projectID"), r.PathValue("dossierID"), input.Version)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dossier)
}

func (s *Server) decideDossier(w http.ResponseWriter, r *http.Request) {
	p, err := principal(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var input struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
		Version int64  `json:"version"`
	}
	if err := s.decode(w, r, &input); err != nil {
		s.writeError(w, r, err)
		return
	}
	dossier, err := s.service.DecideDossier(r.Context(), p, r.PathValue("projectID"), r.PathValue("dossierID"), input.Note, input.Approve, input.Version)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dossier)
}
