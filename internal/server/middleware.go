package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"office-assistant/internal/auth"
)

func (a *api) requireAdmin(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := auth.ClaimsFromContext(r.Context())
		if claims.Role != "admin" {
			writeError(w, http.StatusForbidden, errors.New("admin role is required"))
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a *api) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		tokenValue, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(tokenValue) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("bearer token is required"))
			return
		}
		claims, err := a.tokens.Verify(strings.TrimSpace(tokenValue))
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
	})
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "elapsed_ms", time.Since(start).Milliseconds())
	})
}
