package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	chatpkg "office-assistant/backend/chat"
	"office-assistant/backend/domain"
	"office-assistant/backend/httpapi"
	ingestionpkg "office-assistant/backend/ingestion"
	"office-assistant/backend/providers"
	"office-assistant/backend/utils"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const (
	chatAppName        = "office-assistant"
	retrievalToolName  = "retrieve_knowledge"
	maxRetrievalLimit  = 5
	vectorProbeLimit   = 20
	chatHistoryLimit   = 12
	chatRequestTimeout = 2 * time.Minute
)

type chatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

type citationPreviewResponse struct {
	SessionID           string         `json:"session_id"`
	CitationID          string         `json:"citation_id"`
	DocumentID          int64          `json:"document_id"`
	DocumentName        string         `json:"document_name"`
	HeadingPath         string         `json:"heading_path,omitempty"`
	SourceAnchor        map[string]any `json:"source_anchor,omitempty"`
	Text                string         `json:"text"`
	OriginalDownloadURL string         `json:"original_download_url"`
}

func (a *app) chatKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	var req chatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message_required", "message is required", nil)
		return
	}

	sessionRecord, err := a.resolveChatSession(r.Context(), current, kb, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not prepare chat session", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), chatRequestTimeout)
	defer cancel()
	a.registerChatCancel(sessionRecord.ID, cancel)
	defer a.unregisterChatCancel(sessionRecord.ID)

	emitter := httpapi.NewSSEEmitter(w)
	emit := emitter.Emit

	history, err := a.store.ListAllChatMessages(ctx, sessionRecord.ID)
	if err != nil {
		emit("error", httpapi.APIError{Code: "store_error", Message: "could not load chat history"})
		return
	}
	if err := a.store.AppendChatMessage(ctx, domain.ChatMessage{SessionID: sessionRecord.ID, Role: "user", Content: req.Message, Metadata: "{}"}); err != nil {
		emit("error", httpapi.APIError{Code: "store_error", Message: "could not save chat message"})
		return
	}
	emit("start", map[string]any{"session_id": sessionRecord.ID})

	answer, evidence, retrievalCalled, err := a.runKnowledgeBaseAgent(ctx, current, kb, sessionRecord.ID, req.Message, history, emit)
	if err != nil {
		code := "chat_error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "chat_cancelled"
		}
		_ = a.store.AppendChatMessage(context.Background(), domain.ChatMessage{SessionID: sessionRecord.ID, Role: "error", Content: err.Error(), Metadata: "{}"})
		emit("error", httpapi.APIError{Code: code, Message: err.Error()})
		return
	}
	if !retrievalCalled {
		err := errors.New("knowledge-base answers require retrieval before final response")
		_ = a.store.AppendChatMessage(context.Background(), domain.ChatMessage{SessionID: sessionRecord.ID, Role: "error", Content: err.Error(), Metadata: "{}"})
		emit("error", httpapi.APIError{Code: "retrieval_required", Message: err.Error()})
		return
	}
	if len(evidence) == 0 {
		answer = unsupportedAnswerMessage()
	}
	_ = a.store.RecordMetric(context.Background(), "chat_token_estimate", 0, int64(ingestionpkg.EstimatedTokenCount(req.Message)+ingestionpkg.EstimatedTokenCount(answer)), map[string]any{"session_id": sessionRecord.ID})
	metadata, _ := json.Marshal(map[string]any{"citations": evidence})
	if err := a.store.AppendChatMessage(context.Background(), domain.ChatMessage{SessionID: sessionRecord.ID, Role: "assistant", Content: answer, Metadata: string(metadata)}); err != nil {
		emit("error", httpapi.APIError{Code: "store_error", Message: "could not save assistant message"})
		return
	}
	emit("citations", map[string]any{"citations": evidence})
	emit("done", map[string]any{"session_id": sessionRecord.ID})
}

func (a *app) getCitationPreview(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	sessionID := r.PathValue("id")
	citationID := r.PathValue("citation")
	_, err := chatpkg.FindOwnedSession(r.Context(), a.store, current, sessionID)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "citation_not_found", "citation not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load citation", nil)
		return
	}
	citation, ok, err := a.findPersistedCitation(r.Context(), sessionID, citationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load citation", nil)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "citation_not_found", "citation not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, citationPreviewResponse{
		SessionID:           sessionID,
		CitationID:          citation.CitationID,
		DocumentID:          citation.DocumentID,
		DocumentName:        citation.DocumentName,
		HeadingPath:         citation.HeadingPath,
		SourceAnchor:        citation.SourceAnchor,
		Text:                citation.Text,
		OriginalDownloadURL: "/api/documents/" + strconv.FormatInt(citation.DocumentID, 10) + "/download",
	})
}

func (a *app) cancelChatSession(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	sessionID := r.PathValue("id")
	_, err := chatpkg.FindOwnedSession(r.Context(), a.store, current, sessionID)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat session", nil)
		return
	}
	a.activeChats.Cancel(sessionID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancel_requested"})
}

func (a *app) resolveChatSession(ctx context.Context, current domain.User, kb domain.KnowledgeBase, req chatRequest) (domain.ChatSession, error) {
	return chatpkg.ResolveSession(ctx, a.store, current, kb, req.SessionID, req.Message, randomToken)
}

func (a *app) runKnowledgeBaseAgent(ctx context.Context, current domain.User, kb domain.KnowledgeBase, sessionID, message string, history []domain.ChatMessage, emit func(string, any)) (string, []chatpkg.RetrievalEvidence, bool, error) {
	runner := chatpkg.Runner{
		AppName:           chatAppName,
		RetrievalToolName: retrievalToolName,
		MaxRetrievalLimit: maxRetrievalLimit,
	}
	result, err := runner.Run(ctx, chatpkg.RunRequest{
		Current:   current,
		KB:        kb,
		SessionID: sessionID,
		Message:   message,
	}, chatpkg.RunDeps{
		LoadHistory: func(context.Context, string) ([]domain.ChatMessage, error) {
			return history, nil
		},
		Retrieve: a.retrieveKnowledge,
		Model: func(ctx context.Context) (model.LLM, error) {
			setting, err := a.store.FindProviderSetting(ctx, providerPurposeChat)
			if err != nil {
				return nil, err
			}
			return providers.ChatModel(setting, a.config.fakeProviders, a.httpClient, retrievalToolName, maxRetrievalLimit)
		},
		RecordPrompt: func(ctx context.Context, kb domain.KnowledgeBase, sessionID, prompt string) {
			if !a.debugEnabled(ctx) {
				return
			}
			_ = a.store.PruneDebugTraces(ctx, time.Now())
			_ = a.store.AppendDebugTrace(ctx, correlationID(ctx), "chat_prompt", map[string]any{"session_id": sessionID, "knowledge_base_id": kb.ID, "prompt": prompt}, debugTraceRetention)
		},
		RecordGeneration: func(ctx context.Context, kb domain.KnowledgeBase, sessionID string, duration time.Duration) {
			_ = a.store.RecordMetric(ctx, "generation_latency", duration, 1, map[string]any{"session_id": sessionID, "knowledge_base_id": kb.ID})
		},
		CorrelationID: correlationID,
	}, emit)
	return result.Answer, result.Evidence, result.RetrievalCalled, err
}

func (a *app) retrieveKnowledge(ctx context.Context, kb domain.KnowledgeBase, args chatpkg.RetrievalToolArgs) (chatpkg.RetrievalToolResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return chatpkg.RetrievalToolResult{}, errors.New("retrieval query is required")
	}
	if a.vectorIndex == nil {
		return chatpkg.RetrievalToolResult{}, errors.New("vector index is unavailable")
	}
	limit := args.Limit
	if limit <= 0 || limit > maxRetrievalLimit {
		limit = maxRetrievalLimit
	}
	started := time.Now()
	results, err := a.vectorIndex.Search(ctx, query, vectorProbeLimit)
	if err != nil {
		return chatpkg.RetrievalToolResult{}, err
	}
	out := chatpkg.RetrievalToolResult{Results: make([]chatpkg.RetrievalEvidence, 0, limit)}
	for _, result := range results {
		if len(out.Results) >= limit {
			break
		}
		chunk, err := a.store.FindRetrievalChunk(ctx, result.ChunkID, kb.ID)
		if err != nil {
			if utils.NotFound(err) {
				continue
			}
			return chatpkg.RetrievalToolResult{}, err
		}
		out.Results = append(out.Results, chatpkg.EvidenceFromChunk(len(out.Results), chunk))
	}
	_ = a.store.RecordMetric(ctx, "retrieval_latency", time.Since(started), int64(len(out.Results)), map[string]any{"knowledge_base_id": kb.ID, "query": query})
	if a.debugEnabled(ctx) {
		_ = a.store.AppendDebugTrace(ctx, correlationID(ctx), "retrieval", map[string]any{"knowledge_base_id": kb.ID, "query": query, "result_count": len(out.Results)}, debugTraceRetention)
	}
	return out, nil
}

func (a *app) promptWithHistory(ctx context.Context, sessionID, message string) (string, error) {
	messages, err := a.store.ListChatMessages(ctx, sessionID, chatHistoryLimit)
	if err != nil {
		return "", err
	}
	return chatpkg.PromptWithHistory(messages, message), nil
}

func knowledgeBaseInstruction(kb domain.KnowledgeBase) string {
	return chatpkg.KnowledgeBaseInstruction(kb)
}

func unsupportedAnswerMessage() string {
	return chatpkg.UnsupportedAnswerMessage()
}

func (a *app) findPersistedCitation(ctx context.Context, sessionID, citationID string) (chatpkg.RetrievalEvidence, bool, error) {
	messages, err := a.store.ListChatMessages(ctx, sessionID, 100)
	if err != nil {
		return chatpkg.RetrievalEvidence{}, false, err
	}
	citation, ok := chatpkg.FindPersistedCitation(messages, citationID)
	return citation, ok, nil
}

func visibleText(content *genai.Content) string {
	return chatpkg.VisibleText(content)
}

func (a *app) registerChatCancel(sessionID string, cancel context.CancelFunc) {
	a.activeChats.Register(sessionID, cancel)
}

func (a *app) unregisterChatCancel(sessionID string) {
	a.activeChats.Unregister(sessionID)
}
