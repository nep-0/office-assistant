package server

import (
	"net/http"
)

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := a.store.Ping(r.Context()); err != nil {
		dbStatus = "error"
		a.logger.Warn("database health check failed", "error", err)
	}

	settings, err := a.store.GetProviderSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"database": map[string]string{
			"status": dbStatus,
		},
		"providers": map[string]any{
			"embedding": providerSummary(settings.Embedding),
			"chat":      providerSummary(settings.Chat),
		},
	})
}
