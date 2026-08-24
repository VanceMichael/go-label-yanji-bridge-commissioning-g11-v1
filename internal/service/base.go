package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

type Service struct {
	store repository.Store
	clock repository.Clock
}

func New(store repository.Store, clock repository.Clock) *Service {
	if clock == nil {
		clock = repository.RealClock{}
	}
	return &Service{store: store, clock: clock}
}

func (s *Service) Ready(ctx context.Context) error { return s.store.Ping(ctx) }

func requireRole(principal domain.Principal, roles ...domain.Role) error {
	if !principal.Allows(roles...) {
		return domain.ErrForbidden
	}
	return nil
}

func hashRequest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode idempotency request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func replay(ctx context.Context, repo repository.MutationRepository, scope repository.IdempotencyScope, requestHash string, now time.Time, target any) (bool, int, error) {
	record, err := repo.GetIdempotency(ctx, scope)
	if errors.Is(err, domain.ErrNotFound) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if now.After(record.ExpiresAt) {
		return false, 0, domain.ErrExpired
	}
	if record.RequestHash != requestHash {
		return false, 0, fmt.Errorf("idempotency key reused with another request: %w", domain.ErrConflict)
	}
	if err := json.Unmarshal(record.Response, target); err != nil {
		return false, 0, fmt.Errorf("decode idempotency response: %w", err)
	}
	return true, record.StatusCode, nil
}

func remember(ctx context.Context, repo repository.MutationRepository, scope repository.IdempotencyScope, requestHash string, status int, response any, now time.Time) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return repo.PutIdempotency(ctx, repository.IdempotencyRecord{Scope: scope, RequestHash: requestHash, StatusCode: status, Response: encoded, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)})
}
