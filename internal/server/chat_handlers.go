package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"office-assistant/internal/app"
)

type chatRequest struct {
	SessionID       string `json:"session_id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Message         string `json:"message"`
}

func (a *api) streamChat(w http.ResponseWriter, r *http.Request) {
	var request chatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	final, err := a.chat.Stream(r.Context(), app.ChatInput{
		SessionID:       request.SessionID,
		KnowledgeBaseID: request.KnowledgeBaseID,
		Message:         request.Message,
	}, func(token string) error {
		writeSSE(w, "token", map[string]string{"text": token})
		flusher.Flush()
		return nil
	})
	if err != nil {
		a.logger.Warn("chat stream failed", "error", err)
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	writeSSE(w, "final", map[string]any{
		"session_id":  final.SessionID,
		"message_id":  final.MessageID,
		"elapsed_ms":  final.ElapsedMS,
		"answer":      final.Answer,
		"citations":   final.Citations,
		"provider":    final.Provider,
		"source_type": final.SourceType,
		"tool_calls":  final.ToolCalls,
	})
	flusher.Flush()
}
