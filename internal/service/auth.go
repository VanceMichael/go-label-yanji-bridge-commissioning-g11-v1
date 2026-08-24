package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/idgen"
)

type LoginResult struct {
	Token     string           `json:"token"`
	ExpiresAt time.Time        `json:"expires_at"`
	User      domain.Principal `json:"user"`
}

func (s *Service) Login(ctx context.Context, email, password string, ttl time.Duration) (LoginResult, error) {
	if strings.TrimSpace(email) == "" || password == "" || ttl <= 0 {
		return LoginResult{}, domain.ErrInvalid
	}
	user, err := s.store.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return LoginResult{}, domain.ErrUnauthorized
		}
		return LoginResult{}, fmt.Errorf("find login user: %w", err)
	}
	if !user.DisabledAt.IsZero() || !auth.VerifyPassword(user.PasswordHash, password) {
		return LoginResult{}, domain.ErrUnauthorized
	}
	plain, digest, err := auth.NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.clock.Now()
	session := domain.Session{ID: idgen.New("ses"), UserID: user.ID, Organization: user.Organization, TokenHash: digest, ExpiresAt: now.Add(ttl), CreatedAt: now}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return LoginResult{}, fmt.Errorf("persist login session: %w", err)
	}
	return LoginResult{Token: plain, ExpiresAt: session.ExpiresAt, User: domain.Principal{UserID: user.ID, Organization: user.Organization, Role: user.Role, SessionID: session.ID}}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	if token == "" {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	session, user, err := s.store.FindSessionByHash(ctx, auth.HashToken(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Principal{}, domain.ErrUnauthorized
		}
		return domain.Principal{}, fmt.Errorf("read session: %w", err)
	}
	if err := session.Active(s.clock.Now()); err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	if !user.DisabledAt.IsZero() {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return domain.Principal{UserID: user.ID, Organization: user.Organization, Role: user.Role, SessionID: session.ID}, nil
}

func (s *Service) Logout(ctx context.Context, principal domain.Principal) error {
	if principal.SessionID == "" {
		return domain.ErrUnauthorized
	}
	if err := s.store.RevokeSession(ctx, principal.SessionID, s.clock.Now()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
