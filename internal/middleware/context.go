package middleware

import (
	"context"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

type contextKey string

const (
	requestIDKey contextKey = "request-id"
	principalKey contextKey = "principal"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}
func WithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}
func Principal(ctx context.Context) (domain.Principal, bool) {
	value, ok := ctx.Value(principalKey).(domain.Principal)
	return value, ok
}
