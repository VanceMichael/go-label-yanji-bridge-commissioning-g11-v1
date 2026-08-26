package domain

import "time"

type Role string

const (
	RoleOwnerAdmin         Role = "owner_admin"
	RoleContractorEngineer Role = "contractor_engineer"
	RoleSupervisor         Role = "supervisor"
	RoleCommissioning      Role = "commissioning_officer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOwnerAdmin, RoleContractorEngineer, RoleSupervisor, RoleCommissioning:
		return true
	default:
		return false
	}
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID           string    `json:"id"`
	Organization string    `json:"organization_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	PasswordHash []byte    `json:"-"`
	DisabledAt   time.Time `json:"disabled_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Organization string    `json:"organization_id"`
	TokenHash    string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	RevokedAt    time.Time `json:"revoked_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s Session) Active(now time.Time) error {
	if !s.RevokedAt.IsZero() {
		return ErrUnauthorized
	}
	if !now.Before(s.ExpiresAt) {
		return ErrExpired
	}
	return nil
}

type Principal struct {
	UserID       string `json:"user_id"`
	Organization string `json:"organization_id"`
	Role         Role   `json:"role"`
	SessionID    string `json:"session_id"`
}

func (p Principal) Allows(roles ...Role) bool {
	for _, role := range roles {
		if p.Role == role {
			return true
		}
	}
	return false
}
