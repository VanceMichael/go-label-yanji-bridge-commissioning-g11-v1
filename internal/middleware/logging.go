package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	count, err := w.ResponseWriter.Write(value)
	w.bytes += count
	return count, err
}
func Logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		logger.Info("http request", "request_id", RequestID(r.Context()), "method", r.Method, "path", r.URL.Path, "status", wrapped.status, "bytes", wrapped.bytes, "duration_ms", time.Since(started).Milliseconds())
	})
}
