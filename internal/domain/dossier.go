package domain

import "time"

type DossierStatus string

const (
	DossierAssembling DossierStatus = "assembling"
	DossierReview     DossierStatus = "under_review"
	DossierApproved   DossierStatus = "approved"
	DossierRejected   DossierStatus = "rejected"
)

type HandoverDossier struct {
	ID           string        `json:"id"`
	ProjectID    string        `json:"project_id"`
	Status       DossierStatus `json:"status"`
	SubmittedBy  string        `json:"submitted_by,omitempty"`
	SubmittedAt  time.Time     `json:"submitted_at,omitempty"`
	DecidedBy    string        `json:"decided_by,omitempty"`
	DecidedAt    time.Time     `json:"decided_at,omitempty"`
	DecisionNote string        `json:"decision_note,omitempty"`
	Version      int64         `json:"version"`
}

type DossierEvidence struct {
	RequiredDocuments int `json:"required_documents"`
	PresentDocuments  int `json:"present_documents"`
	OpenFindings      int `json:"open_findings"`
	UnacceptedWork    int `json:"unaccepted_work"`
	FailedInspections int `json:"failed_inspections"`
	PassedLoadRuns    int `json:"passed_load_runs"`
}

func (e DossierEvidence) Ready() error {
	if e.RequiredDocuments == 0 || e.PresentDocuments < e.RequiredDocuments {
		return &StateError{Entity: "dossier", From: string(DossierAssembling), To: string(DossierReview), Reason: "required documents are incomplete"}
	}
	if e.OpenFindings > 0 || e.UnacceptedWork > 0 || e.FailedInspections > 0 {
		return &StateError{Entity: "dossier", From: string(DossierAssembling), To: string(DossierReview), Reason: "closeout blockers remain"}
	}
	if e.PassedLoadRuns == 0 {
		return &StateError{Entity: "dossier", From: string(DossierAssembling), To: string(DossierReview), Reason: "no passed load test exists"}
	}
	return nil
}

func (d *HandoverDossier) Submit(actor string, evidence DossierEvidence, now time.Time) error {
	if d.Status != DossierAssembling {
		return &StateError{Entity: "dossier", From: string(d.Status), To: string(DossierReview), Reason: "dossier is not assembling"}
	}
	if err := evidence.Ready(); err != nil {
		return err
	}
	d.Status, d.SubmittedBy, d.SubmittedAt, d.Version = DossierReview, actor, now.UTC(), d.Version+1
	return nil
}

func (d *HandoverDossier) Decide(actor, note string, approve bool, now time.Time) error {
	if d.Status != DossierReview {
		return &StateError{Entity: "dossier", From: string(d.Status), To: "decision", Reason: "dossier is not under review"}
	}
	if err := Required("decision_note", note); err != nil {
		return err
	}
	if approve {
		d.Status = DossierApproved
	} else {
		d.Status = DossierRejected
	}
	d.DecidedBy, d.DecidedAt, d.DecisionNote, d.Version = actor, now.UTC(), note, d.Version+1
	return nil
}
