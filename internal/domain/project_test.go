package domain

import (
	"errors"
	"testing"
	"time"
)

func TestProjectValidate(t *testing.T) {
	target := time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		project Project
		wantErr bool
	}{
		{
			name: "valid closeout project",
			project: Project{
				Name:         "Yanji bridge commissioning",
				TargetOpenAt: target,
				Timezone:     "Asia/Shanghai",
			},
		},
		{
			name: "missing name",
			project: Project{
				TargetOpenAt: target,
				Timezone:     "Asia/Shanghai",
			},
			wantErr: true,
		},
		{
			name: "missing opening target",
			project: Project{
				Name:     "Yanji bridge commissioning",
				Timezone: "Asia/Shanghai",
			},
			wantErr: true,
		},
		{
			name: "unknown timezone",
			project: Project{
				Name:         "Yanji bridge commissioning",
				TargetOpenAt: target,
				Timezone:     "Mars/Olympus",
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.project.Validate()
			if test.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want invalid", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestProjectAdvanceInOrder(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	project := Project{
		ID:      "project-1",
		Status:  ProjectCloseout,
		Version: 4,
	}
	steps := []struct {
		to          ProjectStatus
		wantVersion int64
	}{
		{to: ProjectTesting, wantVersion: 5},
		{to: ProjectHandover, wantVersion: 6},
		{to: ProjectReady, wantVersion: 7},
	}
	for _, step := range steps {
		if err := project.Advance(step.to, now); err != nil {
			t.Fatalf("Advance(%s) error = %v", step.to, err)
		}
		if project.Status != step.to {
			t.Fatalf("status = %s, want %s", project.Status, step.to)
		}
		if project.Version != step.wantVersion {
			t.Fatalf("version = %d, want %d", project.Version, step.wantVersion)
		}
		if !project.UpdatedAt.Equal(now) {
			t.Fatalf("updated_at = %s, want %s", project.UpdatedAt, now)
		}
	}
}

func TestProjectAdvanceRejectsSkippedAndReversePhases(t *testing.T) {
	tests := []struct {
		name string
		from ProjectStatus
		to   ProjectStatus
	}{
		{name: "closeout cannot skip to handover", from: ProjectCloseout, to: ProjectHandover},
		{name: "closeout cannot skip to ready", from: ProjectCloseout, to: ProjectReady},
		{name: "testing cannot return to closeout", from: ProjectTesting, to: ProjectCloseout},
		{name: "testing cannot skip to ready", from: ProjectTesting, to: ProjectReady},
		{name: "handover cannot return to testing", from: ProjectHandover, to: ProjectTesting},
		{name: "ready cannot return to handover", from: ProjectReady, to: ProjectHandover},
		{name: "ready cannot repeat transition", from: ProjectReady, to: ProjectReady},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := Project{Status: test.from, Version: 8}
			err := project.Advance(test.to, time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Advance() error = %v, want conflict", err)
			}
			if project.Status != test.from {
				t.Fatalf("failed transition changed status to %s", project.Status)
			}
			if project.Version != 8 {
				t.Fatalf("failed transition changed version to %d", project.Version)
			}
		})
	}
}

func TestRoleValidationAndAuthorization(t *testing.T) {
	validRoles := []Role{
		RoleOwnerAdmin,
		RoleContractorEngineer,
		RoleSupervisor,
		RoleCommissioning,
	}
	for _, role := range validRoles {
		if !role.Valid() {
			t.Fatalf("role %q should be valid", role)
		}
		principal := Principal{Role: role}
		if !principal.Allows(role) {
			t.Fatalf("principal role %q should allow itself", role)
		}
		if principal.Allows(Role("unknown")) {
			t.Fatalf("principal role %q allowed unknown role", role)
		}
	}
	invalidRoles := []Role{"", "admin", "viewer", "contractor", "OWNER_ADMIN"}
	for _, role := range invalidRoles {
		if role.Valid() {
			t.Fatalf("role %q should be invalid", role)
		}
	}
}
