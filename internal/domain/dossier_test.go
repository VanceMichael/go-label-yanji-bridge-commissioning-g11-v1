package domain

import (
	"errors"
	"testing"
	"time"
)

func completeEvidence() DossierEvidence {
	return DossierEvidence{
		RequiredDocuments: 8,
		PresentDocuments:  8,
		OpenFindings:      0,
		UnacceptedWork:    0,
		FailedInspections: 0,
		PassedLoadRuns:    2,
	}
}

func TestDossierEvidenceReady(t *testing.T) {
	if err := completeEvidence().Ready(); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
}

func TestDossierEvidenceRejectsEveryBlockingCondition(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DossierEvidence)
	}{
		{name: "required document list missing", mutate: func(e *DossierEvidence) { e.RequiredDocuments = 0; e.PresentDocuments = 0 }},
		{name: "one document missing", mutate: func(e *DossierEvidence) { e.PresentDocuments = e.RequiredDocuments - 1 }},
		{name: "all documents missing", mutate: func(e *DossierEvidence) { e.PresentDocuments = 0 }},
		{name: "one open finding", mutate: func(e *DossierEvidence) { e.OpenFindings = 1 }},
		{name: "several open findings", mutate: func(e *DossierEvidence) { e.OpenFindings = 12 }},
		{name: "one unaccepted package", mutate: func(e *DossierEvidence) { e.UnacceptedWork = 1 }},
		{name: "several unaccepted packages", mutate: func(e *DossierEvidence) { e.UnacceptedWork = 7 }},
		{name: "failed inspection", mutate: func(e *DossierEvidence) { e.FailedInspections = 1 }},
		{name: "several failed inspections", mutate: func(e *DossierEvidence) { e.FailedInspections = 4 }},
		{name: "no passed load run", mutate: func(e *DossierEvidence) { e.PassedLoadRuns = 0 }},
		{name: "documents and findings blocked", mutate: func(e *DossierEvidence) { e.PresentDocuments = 1; e.OpenFindings = 3 }},
		{name: "work and load test blocked", mutate: func(e *DossierEvidence) { e.UnacceptedWork = 2; e.PassedLoadRuns = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := completeEvidence()
			test.mutate(&evidence)
			err := evidence.Ready()
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Ready() error = %v, want conflict", err)
			}
		})
	}
}

func TestDossierSubmit(t *testing.T) {
	now := time.Date(2026, 11, 10, 4, 0, 0, 0, time.UTC)
	dossier := HandoverDossier{ID: "dossier-1", Status: DossierAssembling, Version: 3}
	if err := dossier.Submit("owner-1", completeEvidence(), now); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if dossier.Status != DossierReview {
		t.Fatalf("status = %s, want under_review", dossier.Status)
	}
	if dossier.SubmittedBy != "owner-1" {
		t.Fatalf("submitted_by = %q", dossier.SubmittedBy)
	}
	if !dossier.SubmittedAt.Equal(now) {
		t.Fatalf("submitted_at = %s, want %s", dossier.SubmittedAt, now)
	}
	if dossier.Version != 4 {
		t.Fatalf("version = %d, want 4", dossier.Version)
	}
}

func TestDossierSubmitRollsBackObjectMutationOnEvidenceFailure(t *testing.T) {
	evidence := completeEvidence()
	evidence.OpenFindings = 1
	dossier := HandoverDossier{Status: DossierAssembling, Version: 5}
	err := dossier.Submit("owner-1", evidence, time.Now())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Submit() error = %v, want conflict", err)
	}
	if dossier.Status != DossierAssembling || dossier.SubmittedBy != "" || !dossier.SubmittedAt.IsZero() || dossier.Version != 5 {
		t.Fatalf("failed submission mutated dossier: %+v", dossier)
	}
}

func TestDossierSubmitRejectsWrongState(t *testing.T) {
	states := []DossierStatus{DossierReview, DossierApproved, DossierRejected, "unknown"}
	for _, state := range states {
		dossier := HandoverDossier{Status: state, Version: 2}
		err := dossier.Submit("owner", completeEvidence(), time.Now())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("status %s error = %v, want conflict", state, err)
		}
		if dossier.Status != state || dossier.Version != 2 {
			t.Fatalf("failed submission mutated dossier: %+v", dossier)
		}
	}
}

func TestDossierDecideApproveAndReject(t *testing.T) {
	now := time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		approve bool
		want    DossierStatus
	}{
		{name: "approve", approve: true, want: DossierApproved},
		{name: "reject", approve: false, want: DossierRejected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dossier := HandoverDossier{Status: DossierReview, Version: 8}
			err := dossier.Decide("supervisor-1", "evidence reviewed", test.approve, now)
			if err != nil {
				t.Fatalf("Decide() error = %v", err)
			}
			if dossier.Status != test.want {
				t.Fatalf("status = %s, want %s", dossier.Status, test.want)
			}
			if dossier.DecidedBy != "supervisor-1" || dossier.DecisionNote != "evidence reviewed" {
				t.Fatalf("decision metadata = %+v", dossier)
			}
			if !dossier.DecidedAt.Equal(now) || dossier.Version != 9 {
				t.Fatalf("decision timestamp/version = %+v", dossier)
			}
		})
	}
}

func TestDossierDecideRequiresReviewAndNote(t *testing.T) {
	tests := []struct {
		name   string
		status DossierStatus
		note   string
		kind   error
	}{
		{name: "assembling cannot approve", status: DossierAssembling, note: "ok", kind: ErrConflict},
		{name: "approved cannot decide again", status: DossierApproved, note: "ok", kind: ErrConflict},
		{name: "rejected cannot decide again", status: DossierRejected, note: "ok", kind: ErrConflict},
		{name: "review requires note", status: DossierReview, note: "", kind: ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dossier := HandoverDossier{Status: test.status, Version: 4}
			err := dossier.Decide("supervisor", test.note, true, time.Now())
			if !errors.Is(err, test.kind) {
				t.Fatalf("Decide() error = %v, want %v", err, test.kind)
			}
			if dossier.Status != test.status || dossier.Version != 4 || dossier.DecidedBy != "" || !dossier.DecidedAt.IsZero() {
				t.Fatalf("failed decision mutated dossier: %+v", dossier)
			}
		})
	}
}

func TestSessionActiveLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		session Session
		want    error
	}{
		{name: "active before expiry", session: Session{ExpiresAt: now.Add(time.Hour)}},
		{name: "active one nanosecond before expiry", session: Session{ExpiresAt: now.Add(time.Nanosecond)}},
		{name: "expired exactly now", session: Session{ExpiresAt: now}, want: ErrExpired},
		{name: "expired before now", session: Session{ExpiresAt: now.Add(-time.Hour)}, want: ErrExpired},
		{name: "revoked before expiry", session: Session{ExpiresAt: now.Add(time.Hour), RevokedAt: now.Add(-time.Minute)}, want: ErrUnauthorized},
		{name: "revocation wins over expiry", session: Session{ExpiresAt: now.Add(-time.Hour), RevokedAt: now.Add(-2 * time.Hour)}, want: ErrUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.session.Active(now)
			if test.want == nil && err != nil {
				t.Fatalf("Active() error = %v", err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Active() error = %v, want %v", err, test.want)
			}
		})
	}
}
