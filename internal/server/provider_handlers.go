package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"office-assistant/internal/provider"
	"office-assistant/internal/storage"
)

func (a *api) getProviderSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.GetProviderSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, storage.Masked(settings))
}

func (a *api) putProviderSettings(w http.ResponseWriter, r *http.Request) {
	var settings storage.ProviderSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.store.SaveProviderSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, storage.Masked(settings))
}

func buildChatProvider(slot storage.ProviderSlot) provider.ChatProvider {
	if slot.Kind == "mock" || strings.TrimSpace(slot.BaseURL) == "" {
		return provider.StaticChatProvider{}
	}
	return provider.OpenAIChatProvider{
		Client: provider.EinoOpenAICompatible{
			BaseURL: slot.BaseURL,
			APIKey:  slot.APIKey,
		},
		Model: slot.Model,
	}
}

func providerSummary(slot storage.ProviderSlot) map[string]string {
	configured := "true"
	if slot.Kind == "mock" || strings.TrimSpace(slot.BaseURL) == "" {
		configured = "false"
	}
	return map[string]string{
		"kind":       slot.Kind,
		"base_url":   slot.BaseURL,
		"model":      slot.Model,
		"configured": configured,
	}
}
