package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"office-assistant/backend/domain"
	providerpkg "office-assistant/backend/providers"
)

const (
	providerPurposeChat      = providerpkg.PurposeChat
	providerPurposeEmbedding = providerpkg.PurposeEmbedding
)

type providerSettingResponse struct {
	Purpose    string `json:"purpose"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	APIKeySet  bool   `json:"api_key_set"`
	APIKeyMask string `json:"api_key_mask,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

type providerSettingsResponse struct {
	Settings []providerSettingResponse `json:"settings"`
}

type updateProviderSettingRequest struct {
	BaseURL     string  `json:"base_url"`
	Model       string  `json:"model"`
	APIKey      *string `json:"api_key,omitempty"`
	ClearAPIKey bool    `json:"clear_api_key,omitempty"`
}

func (a *app) getProviderSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	settings, err := a.store.ListProviderSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load provider settings", nil)
		return
	}

	res := providerSettingsResponse{Settings: make([]providerSettingResponse, 0, len(settings))}
	for _, setting := range settings {
		res.Settings = append(res.Settings, maskProviderSetting(setting))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) updateProviderSetting(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	purpose := r.PathValue("purpose")
	if !validProviderPurpose(purpose) {
		writeError(w, http.StatusNotFound, "provider_not_found", "provider purpose not found", nil)
		return
	}

	var req updateProviderSettingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}

	current, err := a.store.FindProviderSetting(r.Context(), purpose)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load provider setting", nil)
		return
	}

	next := domain.ProviderSetting{
		Purpose: purpose,
		BaseURL: strings.TrimSpace(req.BaseURL),
		Model:   strings.TrimSpace(req.Model),
		APIKey:  current.APIKey,
	}
	if req.ClearAPIKey {
		next.APIKey = ""
	} else if req.APIKey != nil {
		next.APIKey = strings.TrimSpace(*req.APIKey)
	}

	if err := validateProviderSetting(next); err != nil {
		writeError(w, http.StatusBadRequest, err.code, err.message, nil)
		return
	}

	updated, err := a.store.UpdateProviderSetting(r.Context(), next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not update provider setting", nil)
		return
	}
	_ = a.store.AppendActivity(r.Context(), currentUser.ID, "provider_setting_changed", "provider", purpose, map[string]any{"base_url": updated.BaseURL, "model": updated.Model, "api_key_set": updated.APIKey != ""})
	writeJSON(w, http.StatusOK, maskProviderSetting(updated))
}

func (a *app) providerDependencyStatus(purpose string) dependencyStatus {
	setting, err := a.store.FindProviderSetting(context.Background(), purpose)
	if err != nil {
		return dependencyStatus{Status: "degraded", Mode: "missing"}
	}
	if strings.TrimSpace(setting.BaseURL) == "" || strings.TrimSpace(setting.Model) == "" {
		return dependencyStatus{Status: "degraded", Mode: "unconfigured"}
	}
	mode := "openai-compatible"
	if a.config.fakeProviders && strings.Contains(setting.BaseURL, "/fake-openai") {
		mode = "fake"
	}
	return dependencyStatus{
		Status: "ready",
		URL:    setting.BaseURL,
		Mode:   mode,
	}
}

type providerValidationError struct {
	code    string
	message string
}

func (err providerValidationError) Error() string {
	return err.code
}

func validateProviderSetting(setting domain.ProviderSetting) *providerValidationError {
	if err := providerpkg.ValidateSetting(setting); err != nil {
		return &providerValidationError{code: err.Code, message: err.Message}
	}
	return nil
}

func validProviderPurpose(purpose string) bool {
	return providerpkg.ValidPurpose(purpose)
}

func maskProviderSetting(setting domain.ProviderSetting) providerSettingResponse {
	return providerSettingResponse{
		Purpose:    setting.Purpose,
		BaseURL:    setting.BaseURL,
		Model:      setting.Model,
		APIKeySet:  setting.APIKey != "",
		APIKeyMask: providerpkg.MaskSecret(setting.APIKey),
		UpdatedAt:  setting.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
