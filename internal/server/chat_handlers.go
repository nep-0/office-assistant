package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"office-assistant/internal/provider"
	"office-assistant/internal/storage"
)

type chatRequest struct {
	SessionID       string `json:"session_id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Message         string `json:"message"`
}

type chunk struct {
	DocumentID     string
	ChunkID        string
	SourceFileName string
	PageNumber     int
	Content        string
	Preview        string
	Score          float64
}

type citationResponse struct {
	ID             string  `json:"id"`
	DocumentID     string  `json:"document_id"`
	ChunkID        string  `json:"chunk_id"`
	SourceFileName string  `json:"source_file_name"`
	PageNumber     int     `json:"page_number"`
	Preview        string  `json:"preview"`
	Score          float64 `json:"score"`
}

func (a *api) streamChat(w http.ResponseWriter, r *http.Request) {
	var request chatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		writeError(w, http.StatusBadRequest, errors.New("message is required"))
		return
	}
	if request.SessionID == "" {
		request.SessionID = newID("session")
	}
	if request.KnowledgeBaseID == "" {
		request.KnowledgeBaseID = "demo"
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	userMessageID := newID("msg")
	assistantMessageID := newID("msg")

	if err := a.store.SaveChatMessage(r.Context(), request.SessionID, userMessageID, "user", request.Message); err != nil {
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	chunk := staticChunk()
	settings, err := a.store.GetProviderSettings(r.Context())
	if err != nil {
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	chatProvider := buildChatProvider(settings.Chat)
	answer := strings.Builder{}
	start := time.Now()
	err = chatProvider.Stream(r.Context(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "system", Content: "Answer from the provided context. If the context is insufficient, say so. Keep the answer concise and cite the supporting source."},
			{Role: "user", Content: fmt.Sprintf("Context:\n%s\n\nQuestion: %s", chunk.Content, request.Message)},
		},
	}, func(token string) error {
		answer.WriteString(token)
		writeSSE(w, "token", map[string]string{"text": token})
		flusher.Flush()
		return nil
	})
	if err != nil {
		a.logger.Warn("chat provider failed", "provider", settings.Chat.Kind, "error", err)
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	if err := a.store.SaveChatMessage(r.Context(), request.SessionID, assistantMessageID, "assistant", answer.String()); err != nil {
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	citationID := newID("cit")
	citation := storage.CitationRecord{
		ID:             citationID,
		MessageID:      assistantMessageID,
		DocumentID:     chunk.DocumentID,
		ChunkID:        chunk.ChunkID,
		SourceFileName: chunk.SourceFileName,
		PageNumber:     &chunk.PageNumber,
		Preview:        chunk.Preview,
		Score:          &chunk.Score,
	}
	if err := a.store.SaveCitation(r.Context(), citation); err != nil {
		writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}

	writeSSE(w, "final", map[string]any{
		"session_id":  request.SessionID,
		"message_id":  assistantMessageID,
		"elapsed_ms":  time.Since(start).Milliseconds(),
		"answer":      answer.String(),
		"citations":   []citationResponse{chunk.toCitation(citationID)},
		"provider":    providerSummary(settings.Chat),
		"source_type": "static-spike-chunk",
	})
	flusher.Flush()
}

func staticChunk() chunk {
	return chunk{
		DocumentID:     "doc_spike",
		ChunkID:        "chunk_spike_001",
		SourceFileName: "spike-source.pdf",
		PageNumber:     1,
		Content:        "The office assistant indexes local documents, answers questions from retrieved chunks, and returns citations for every factual answer.",
		Preview:        "The office assistant indexes local documents, answers questions from retrieved chunks, and returns citations...",
		Score:          1,
	}
}

func (c chunk) toCitation(id string) citationResponse {
	return citationResponse{
		ID:             id,
		DocumentID:     c.DocumentID,
		ChunkID:        c.ChunkID,
		SourceFileName: c.SourceFileName,
		PageNumber:     c.PageNumber,
		Preview:        c.Preview,
		Score:          c.Score,
	}
}
