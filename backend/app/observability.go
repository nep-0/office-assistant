package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"office-assistant/backend/domain"
	"office-assistant/backend/httpapi"
)

const debugTraceRetention = 24 * time.Hour

type activityResponse struct {
	Events []activityEventResponse `json:"events"`
}

type activityEventResponse struct {
	ID         int64          `json:"id"`
	UserID     int64          `json:"user_id,omitempty"`
	EventType  string         `json:"event_type"`
	EntityType string         `json:"entity_type,omitempty"`
	EntityID   string         `json:"entity_id,omitempty"`
	Details    map[string]any `json:"details"`
	CreatedAt  string         `json:"created_at"`
}

type metricsResponse struct {
	Metrics []metricResponse `json:"metrics"`
}

type metricResponse struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	ValueMS   int64          `json:"value_ms"`
	Count     int64          `json:"count,omitempty"`
	Details   map[string]any `json:"details"`
	CreatedAt string         `json:"created_at"`
}

type debugModeResponse struct {
	Enabled           bool   `json:"enabled"`
	Source            string `json:"source"`
	RetentionHours    int    `json:"retention_hours"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	EnvironmentLocked bool   `json:"environment_locked"`
}

type updateDebugModeRequest struct {
	Enabled bool `json:"enabled"`
}

var correlationID = httpapi.CorrelationID

func (a *app) getActivity(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	events, err := a.store.ListActivity(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load activity", nil)
		return
	}
	res := activityResponse{Events: make([]activityEventResponse, 0, len(events))}
	for _, event := range events {
		res.Events = append(res.Events, activityEventResponse{
			ID:         event.ID,
			UserID:     event.UserID,
			EventType:  event.EventType,
			EntityType: event.EntityType,
			EntityID:   event.EntityID,
			Details:    decodeDetails(event.Details),
			CreatedAt:  event.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) getMetrics(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	metrics, err := a.store.ListMetrics(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load metrics", nil)
		return
	}
	res := metricsResponse{Metrics: make([]metricResponse, 0, len(metrics))}
	for _, metric := range metrics {
		res.Metrics = append(res.Metrics, metricResponse{
			ID:        metric.ID,
			Name:      metric.Name,
			ValueMS:   metric.ValueMS,
			Count:     metric.Count,
			Details:   decodeDetails(metric.Details),
			CreatedAt: metric.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) getDebugMode(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	setting, err := a.store.DebugSetting(r.Context(), a.config.debugEnvEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load debug mode", nil)
		return
	}
	writeJSON(w, http.StatusOK, toDebugModeResponse(setting, a.config.debugEnvEnabled))
}

func (a *app) updateDebugMode(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if a.config.debugEnvEnabled {
		writeError(w, http.StatusBadRequest, "debug_mode_environment_locked", "debug mode is controlled by environment", nil)
		return
	}
	var req updateDebugModeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	if err := a.store.SetDebugMode(r.Context(), req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not update debug mode", nil)
		return
	}
	_ = a.store.AppendActivity(r.Context(), current.ID, "debug_mode_changed", "debug", "", map[string]any{"enabled": req.Enabled})
	setting, err := a.store.DebugSetting(r.Context(), a.config.debugEnvEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load debug mode", nil)
		return
	}
	writeJSON(w, http.StatusOK, toDebugModeResponse(setting, a.config.debugEnvEnabled))
}

func toDebugModeResponse(setting domain.DebugSetting, envLocked bool) debugModeResponse {
	res := debugModeResponse{
		Enabled:           setting.Enabled,
		Source:            setting.Source,
		RetentionHours:    int(debugTraceRetention.Hours()),
		EnvironmentLocked: envLocked,
	}
	if !setting.UpdatedAt.IsZero() {
		res.UpdatedAt = setting.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return res
}

func decodeDetails(raw string) map[string]any {
	var details map[string]any
	if json.Unmarshal([]byte(raw), &details) != nil || details == nil {
		return map[string]any{}
	}
	return details
}

func (a *app) debugEnabled(ctx context.Context) bool {
	setting, err := a.store.DebugSetting(ctx, a.config.debugEnvEnabled)
	return err == nil && setting.Enabled
}
