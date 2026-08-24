package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

type BootstrapUser struct {
	ID, Email, DisplayName, Password string
	Role                             domain.Role
}

func (d *DB) Bootstrap(ctx context.Context, organizationID, organizationName string, users []BootstrapUser, now time.Time) error {
	if len(users) < 2 {
		return fmt.Errorf("bootstrap requires at least two role-distinct users")
	}
	return d.WithinTx(ctx, func(repo repository.MutationRepository) error {
		q := repo.(*txRepo).q
		if _, err := q.ExecContext(ctx, `INSERT INTO organizations(id,name,created_at) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`, organizationID, organizationName, timestamp(now)); err != nil {
			return fmt.Errorf("bootstrap organization: %w", err)
		}
		roles := map[domain.Role]bool{}
		for _, user := range users {
			if !user.Role.Valid() {
				return fmt.Errorf("bootstrap user %s: invalid role", user.Email)
			}
			roles[user.Role] = true
			hash, err := auth.HashPassword(user.Password)
			if err != nil {
				return fmt.Errorf("bootstrap user %s: %w", user.Email, err)
			}
			_, err = q.ExecContext(ctx, `INSERT INTO users(id,organization_id,email,display_name,role,password_hash,disabled_at,created_at) VALUES(?,?,?,?,?,?,NULL,?) ON CONFLICT(organization_id,email) DO NOTHING`, user.ID, organizationID, strings.ToLower(user.Email), user.DisplayName, user.Role, hash, timestamp(now))
			if err != nil {
				return fmt.Errorf("bootstrap user %s: %w", user.Email, err)
			}
		}
		if len(roles) < 2 {
			return fmt.Errorf("bootstrap users must have different business roles")
		}
		return nil
	})
}
