package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func (d *DB) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return findUserByEmail(ctx, d.queryer(), email)
}
func (t *txRepo) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return findUserByEmail(ctx, t.queryer(), email)
}
func findUserByEmail(ctx context.Context, q querier, email string) (domain.User, error) {
	row := q.QueryRowContext(ctx, `SELECT id,organization_id,email,display_name,role,password_hash,COALESCE(disabled_at,''),created_at FROM users WHERE email=? COLLATE NOCASE`, email)
	return scanUser(row)
}

func (d *DB) FindUser(ctx context.Context, id string) (domain.User, error) {
	return findUser(ctx, d.queryer(), id)
}
func (t *txRepo) FindUser(ctx context.Context, id string) (domain.User, error) {
	return findUser(ctx, t.queryer(), id)
}
func findUser(ctx context.Context, q querier, id string) (domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, `SELECT id,organization_id,email,display_name,role,password_hash,COALESCE(disabled_at,''),created_at FROM users WHERE id=?`, id))
}

func scanUser(row scanner) (domain.User, error) {
	var user domain.User
	var disabled, created string
	if err := row.Scan(&user.ID, &user.Organization, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash, &disabled, &created); err != nil {
		return domain.User{}, mapNotFound(err)
	}
	var err error
	if user.DisabledAt, err = parseTime(disabled); err != nil {
		return domain.User{}, err
	}
	if user.CreatedAt, err = parseTime(created); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (d *DB) CreateSession(ctx context.Context, session domain.Session) error {
	return createSession(ctx, d.queryer(), session)
}
func (t *txRepo) CreateSession(ctx context.Context, session domain.Session) error {
	return createSession(ctx, t.queryer(), session)
}
func createSession(ctx context.Context, q querier, s domain.Session) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sessions(id,user_id,organization_id,token_hash,expires_at,revoked_at,created_at) VALUES(?,?,?,?,?,?,?)`,
		s.ID, s.UserID, s.Organization, s.TokenHash, timestamp(s.ExpiresAt), timestamp(s.RevokedAt), timestamp(s.CreatedAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (d *DB) FindSessionByHash(ctx context.Context, hash string) (domain.Session, domain.User, error) {
	return findSession(ctx, d.queryer(), hash)
}
func (t *txRepo) FindSessionByHash(ctx context.Context, hash string) (domain.Session, domain.User, error) {
	return findSession(ctx, t.queryer(), hash)
}
func findSession(ctx context.Context, q querier, hash string) (domain.Session, domain.User, error) {
	row := q.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.organization_id,s.token_hash,s.expires_at,COALESCE(s.revoked_at,''),s.created_at,u.id,u.organization_id,u.email,u.display_name,u.role,u.password_hash,COALESCE(u.disabled_at,''),u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, hash)
	var session domain.Session
	var user domain.User
	var expires, revoked, sessionCreated, disabled, userCreated string
	if err := row.Scan(&session.ID, &session.UserID, &session.Organization, &session.TokenHash, &expires, &revoked, &sessionCreated,
		&user.ID, &user.Organization, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash, &disabled, &userCreated); err != nil {
		return domain.Session{}, domain.User{}, mapNotFound(err)
	}
	var err error
	if session.ExpiresAt, err = parseTime(expires); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if session.RevokedAt, err = parseTime(revoked); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if session.CreatedAt, err = parseTime(sessionCreated); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if user.DisabledAt, err = parseTime(disabled); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	if user.CreatedAt, err = parseTime(userCreated); err != nil {
		return domain.Session{}, domain.User{}, err
	}
	return session, user, nil
}

func (d *DB) RevokeSession(ctx context.Context, id string, now time.Time) error {
	return revokeSession(ctx, d.queryer(), id, now)
}
func (t *txRepo) RevokeSession(ctx context.Context, id string, now time.Time) error {
	return revokeSession(ctx, t.queryer(), id, now)
}
func revokeSession(ctx context.Context, q querier, id string, now time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, timestamp(now), id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		var exists int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id=?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return domain.ErrNotFound
		}
	}
	return nil
}

var _ = sql.ErrNoRows
