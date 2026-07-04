package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	providerPurposeChat      = "chat"
	providerPurposeEmbedding = "embedding"
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
	settings, err := a.store.listProviderSettings(r.Context())
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

	current, err := a.store.findProviderSetting(r.Context(), purpose)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load provider setting", nil)
		return
	}

	next := providerSetting{
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

	updated, err := a.store.updateProviderSetting(r.Context(), next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not update provider setting", nil)
		return
	}
	_ = a.store.appendActivity(r.Context(), currentUser.ID, "provider_setting_changed", "provider", purpose, map[string]any{"base_url": updated.BaseURL, "model": updated.Model, "api_key_set": updated.APIKey != ""})
	writeJSON(w, http.StatusOK, maskProviderSetting(updated))
}

func (a *app) providerDependencyStatus(purpose string) dependencyStatus {
	setting, err := a.store.findProviderSetting(context.Background(), purpose)
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

func validateProviderSetting(setting providerSetting) *providerValidationError {
	if !validProviderPurpose(setting.Purpose) {
		return &providerValidationError{code: "provider_not_found", message: "provider purpose not found"}
	}
	if strings.TrimSpace(setting.BaseURL) == "" {
		return &providerValidationError{code: "provider_base_url_required", message: "provider base URL is required"}
	}
	parsed, err := url.Parse(setting.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return &providerValidationError{code: "provider_base_url_invalid", message: "provider base URL must be an absolute HTTP URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &providerValidationError{code: "provider_base_url_invalid", message: "provider base URL must use HTTP or HTTPS"}
	}
	if strings.TrimSpace(setting.Model) == "" {
		return &providerValidationError{code: "provider_model_required", message: "provider model is required"}
	}
	return nil
}

func validProviderPurpose(purpose string) bool {
	return purpose == providerPurposeChat || purpose == providerPurposeEmbedding
}

func maskProviderSetting(setting providerSetting) providerSettingResponse {
	return providerSettingResponse{
		Purpose:    setting.Purpose,
		BaseURL:    setting.BaseURL,
		Model:      setting.Model,
		APIKeySet:  setting.APIKey != "",
		APIKeyMask: maskSecret(setting.APIKey),
		UpdatedAt:  setting.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}
