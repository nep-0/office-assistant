package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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

	"github.com/nep-0/harness/agent"
)

const (
	maxRetrievalLimit = 5
	vectorProbeLimit  = 20
	maxHarnessTurns   = 3
	modelContextTurns = 6
)

type chatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

type chatSessionListResponse struct {
	Sessions []chatSessionResponse `json:"sessions"`
}

type chatSessionDetailResponse struct {
	Session  chatSessionResponse   `json:"session"`
	Messages []chatMessageResponse `json:"messages"`
}

type chatSessionResponse struct {
	ID              string `json:"id"`
	KnowledgeBaseID int64  `json:"knowledge_base_id"`
	Title           string `json:"title"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type chatMessageResponse struct {
	ID        int64                       `json:"id"`
	Role      string                      `json:"role"`
	Content   string                      `json:"content"`
	Citations []chatpkg.RetrievalEvidence `json:"citations,omitempty"`
	CreatedAt string                      `json:"created_at"`
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

func (a *app) listChatSessions(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	sessions, err := a.store.ListChatSessionsForKnowledgeBase(r.Context(), current.ID, kb.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat sessions", nil)
		return
	}
	res := chatSessionListResponse{Sessions: make([]chatSessionResponse, 0, len(sessions))}
	for _, session := range sessions {
		res.Sessions = append(res.Sessions, toChatSessionResponse(session))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) getChatSession(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	sessionID := r.PathValue("id")
	sessionRecord, err := chatpkg.FindOwnedSession(r.Context(), a.store, current, sessionID)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat session", nil)
		return
	}
	messages, err := a.store.ListAllChatMessages(r.Context(), sessionRecord.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat messages", nil)
		return
	}
	res := chatSessionDetailResponse{
		Session:  toChatSessionResponse(sessionRecord),
		Messages: make([]chatMessageResponse, 0, len(messages)),
	}
	for _, msg := range messages {
		if !chatpkg.IsVisibleMessage(msg) {
			continue
		}
		res.Messages = append(res.Messages, toChatMessageResponse(msg))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) deleteChatSession(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	sessionID := r.PathValue("id")
	if _, err := chatpkg.FindOwnedSession(r.Context(), a.store, current, sessionID); err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat session", nil)
		return
	}
	if err := a.store.DeleteChatSession(r.Context(), sessionID); err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not delete chat session", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
	ctx, cancel := context.WithTimeout(r.Context(), a.chatRequestTimeout())
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
	emit("start", map[string]any{"session_id": sessionRecord.ID})

	result, err := a.runKnowledgeBaseAgent(ctx, current.Username, kb, sessionRecord.ID, req.Message, history, emit)
	if err != nil {
		result.NewMessages = chatpkg.WithoutFinalAssistant(result.NewMessages)
	}
	if err == nil && result.RetrievalCalled && len(result.Evidence) == 0 {
		result.Answer = unsupportedAnswerMessage()
		for index := len(result.NewMessages) - 1; index >= 0; index-- {
			if result.NewMessages[index].Role == agent.RoleAssistant && len(result.NewMessages[index].ToolCalls) == 0 {
				result.NewMessages[index].Content = result.Answer
				break
			}
		}
	}
	persisted, persistErr := chatpkg.PersistedMessages(sessionRecord.ID, result.NewMessages, result.Evidence)
	if persistErr != nil {
		emit("error", httpapi.APIError{Code: "store_error", Message: "could not encode chat transcript"})
		return
	}
	if err != nil {
		code := "chat_error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "chat_cancelled"
		}
		persisted = append(persisted, domain.ChatMessage{SessionID: sessionRecord.ID, Role: "error", Content: err.Error(), ToolCallsJSON: "[]", Metadata: "{}"})
		if storeErr := a.store.AppendChatMessages(context.Background(), persisted); storeErr != nil {
			emit("error", httpapi.APIError{Code: "store_error", Message: "could not save failed chat transcript"})
			return
		}
		emit("error", httpapi.APIError{Code: code, Message: err.Error()})
		return
	}
	if !result.RetrievalCalled {
		result.Evidence = nil
	}
	if err := a.store.AppendChatMessages(context.Background(), persisted); err != nil {
		emit("error", httpapi.APIError{Code: "store_error", Message: "could not save chat transcript"})
		return
	}
	_ = a.store.RecordMetric(context.Background(), "chat_token_estimate", 0, int64(ingestionpkg.EstimatedTokenCount(req.Message)+ingestionpkg.EstimatedTokenCount(result.Answer)), map[string]any{"session_id": sessionRecord.ID})
	emit("citations", map[string]any{"citations": result.Evidence})
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

func (a *app) runKnowledgeBaseAgent(ctx context.Context, username string, kb domain.KnowledgeBase, sessionID, message string, history []domain.ChatMessage, emit func(string, any)) (chatpkg.RunResult, error) {
	setting, err := a.store.FindProviderSetting(ctx, providerPurposeChat)
	if err != nil {
		return chatpkg.RunResult{}, err
	}
	client := a.chatClient()
	if a.config.fakeProviders && strings.Contains(setting.BaseURL, "/fake-openai") {
		client = providers.FakeChatHTTPClient()
	}
	correlation := correlationID(ctx)
	started := time.Now()
	log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_started", correlation, sessionID)
	result, err := chatpkg.Run(ctx, chatpkg.RunRequest{
		Provider:     setting,
		HTTPClient:   client,
		Instruction:  knowledgeBaseInstruction(kb),
		History:      history,
		Message:      message,
		MaxTurns:     maxHarnessTurns,
		ContextTurns: modelContextTurns,
		Username:     username,
		CurrentDate:  time.Now().UTC().Format(time.DateOnly),
		Retrieve: func(ctx context.Context, args chatpkg.RetrievalToolArgs) (chatpkg.RetrievalToolResult, error) {
			return a.retrieveKnowledge(ctx, kb, args)
		},
		OnTextDelta: func(text string) {
			emit("delta", map[string]any{"text": text})
		},
		OnRetrieval: func(args chatpkg.RetrievalToolArgs) {
			emit("retrieval", map[string]any{"query": args.Query})
		},
		OnPrompt: func(prompt agent.Transcript) {
			if !a.debugEnabled(ctx) {
				return
			}
			_ = a.store.PruneDebugTraces(ctx, time.Now())
			_ = a.store.AppendDebugTrace(ctx, correlationID(ctx), "chat_prompt", map[string]any{"session_id": sessionID, "knowledge_base_id": kb.ID, "prompt": prompt}, debugTraceRetention)
		},
		OnGeneration: func(duration time.Duration) {
			_ = a.store.RecordMetric(ctx, "generation_latency", duration, 1, map[string]any{"session_id": sessionID, "knowledge_base_id": kb.ID})
		},
	})
	if err != nil {
		log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_error error=%q", correlation, sessionID, err.Error())
		return result, err
	}
	log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_completed duration_ms=%d", correlation, sessionID, time.Since(started).Milliseconds())
	return result, nil
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

func knowledgeBaseInstruction(kb domain.KnowledgeBase) string {
	return chatpkg.KnowledgeBaseInstruction(kb)
}

func unsupportedAnswerMessage() string {
	return chatpkg.UnsupportedAnswerMessage()
}

func toChatSessionResponse(session domain.ChatSession) chatSessionResponse {
	return chatSessionResponse{
		ID:              session.ID,
		KnowledgeBaseID: session.KnowledgeBaseID,
		Title:           session.Title,
		CreatedAt:       session.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       session.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toChatMessageResponse(msg domain.ChatMessage) chatMessageResponse {
	res := chatMessageResponse{
		ID:        msg.ID,
		Role:      msg.Role,
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339),
	}
	if msg.Role == "assistant" {
		var metadata struct {
			Citations []chatpkg.RetrievalEvidence `json:"citations"`
		}
		if err := json.Unmarshal([]byte(msg.Metadata), &metadata); err == nil {
			res.Citations = metadata.Citations
		}
	}
	return res
}

func (a *app) findPersistedCitation(ctx context.Context, sessionID, citationID string) (chatpkg.RetrievalEvidence, bool, error) {
	messages, err := a.store.ListChatMessages(ctx, sessionID, 100)
	if err != nil {
		return chatpkg.RetrievalEvidence{}, false, err
	}
	citation, ok := chatpkg.FindPersistedCitation(messages, citationID)
	return citation, ok, nil
}

func (a *app) registerChatCancel(sessionID string, cancel context.CancelFunc) {
	a.activeChats.Register(sessionID, cancel)
}

func (a *app) unregisterChatCancel(sessionID string) {
	a.activeChats.Unregister(sessionID)
}
