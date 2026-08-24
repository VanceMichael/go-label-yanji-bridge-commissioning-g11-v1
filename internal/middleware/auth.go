package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

type Authenticator interface {
	Authenticate(context.Context, string) (domain.Principal, error)
}

type ReadCachingAuthenticator interface {
	AuthenticateCached(context.Context, string) (domain.Principal, error)
}

type ErrorWriter func(http.ResponseWriter, *http.Request, error)

func RequireAuth(authenticator Authenticator, writeError ErrorWriter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			writeError(w, r, domain.ErrUnauthorized)
			return
		}
		plainToken := strings.TrimSpace(token)
		var principal domain.Principal
		var err error
		if cached, ok := authenticator.(ReadCachingAuthenticator); ok && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			principal, err = cached.AuthenticateCached(r.Context(), plainToken)
		} else {
			principal, err = authenticator.Authenticate(r.Context(), plainToken)
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}
