package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
)

func Recover(logger *slog.Logger, writeError ErrorWriter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("recovered http panic", "request_id", RequestID(r.Context()), "panic", fmt.Sprint(recovered))
				writeError(w, r, fmt.Errorf("internal panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
