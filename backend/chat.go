package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	genaiopenai "github.com/achetronic/adk-utils-go/genai/openai"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
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

type retrievalToolArgs struct {
	Query string `json:"query" jsonschema:"Focused search query proposed by the model."`
	Limit int    `json:"limit,omitempty" jsonschema:"Optional desired result count. The backend enforces a maximum."`
}

type retrievalToolResult struct {
	Results []retrievalEvidence `json:"results"`
}

type retrievalEvidence struct {
	CitationID   string         `json:"citation_id"`
	DocumentID   int64          `json:"document_id"`
	DocumentName string         `json:"document_name"`
	ChunkID      int64          `json:"chunk_id"`
	HeadingPath  string         `json:"heading_path,omitempty"`
	SourceAnchor map[string]any `json:"source_anchor,omitempty"`
	Text         string         `json:"text"`
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	emit := func(event string, payload any) {
		data, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	if err := a.store.appendChatMessage(ctx, chatMessage{SessionID: sessionRecord.ID, Role: "user", Content: req.Message, Metadata: "{}"}); err != nil {
		emit("error", apiError{Code: "store_error", Message: "could not save chat message"})
		return
	}
	emit("start", map[string]any{"session_id": sessionRecord.ID})

	answer, evidence, retrievalCalled, err := a.runKnowledgeBaseAgent(ctx, current, kb, sessionRecord.ID, req.Message, emit)
	if err != nil {
		code := "chat_error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "chat_cancelled"
		}
		_ = a.store.appendChatMessage(context.Background(), chatMessage{SessionID: sessionRecord.ID, Role: "error", Content: err.Error(), Metadata: "{}"})
		emit("error", apiError{Code: code, Message: err.Error()})
		return
	}
	if !retrievalCalled {
		err := errors.New("knowledge-base answers require retrieval before final response")
		_ = a.store.appendChatMessage(context.Background(), chatMessage{SessionID: sessionRecord.ID, Role: "error", Content: err.Error(), Metadata: "{}"})
		emit("error", apiError{Code: "retrieval_required", Message: err.Error()})
		return
	}
	if len(evidence) == 0 {
		answer = unsupportedAnswerMessage()
	}
	metadata, _ := json.Marshal(map[string]any{"citations": evidence})
	if err := a.store.appendChatMessage(context.Background(), chatMessage{SessionID: sessionRecord.ID, Role: "assistant", Content: answer, Metadata: string(metadata)}); err != nil {
		emit("error", apiError{Code: "store_error", Message: "could not save assistant message"})
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
	sessionRecord, err := a.store.findChatSession(r.Context(), sessionID)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "citation_not_found", "citation not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load citation", nil)
		return
	}
	if sessionRecord.UserID != current.ID {
		writeError(w, http.StatusNotFound, "citation_not_found", "citation not found", nil)
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
	sessionRecord, err := a.store.findChatSession(r.Context(), sessionID)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load chat session", nil)
		return
	}
	if sessionRecord.UserID != current.ID {
		writeError(w, http.StatusNotFound, "chat_session_not_found", "chat session not found", nil)
		return
	}
	a.activeChatsMu.Lock()
	cancel := a.activeChats[sessionID]
	a.activeChatsMu.Unlock()
	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancel_requested"})
}

func (a *app) resolveChatSession(ctx context.Context, current user, kb knowledgeBase, req chatRequest) (chatSession, error) {
	if req.SessionID != "" {
		sessionRecord, err := a.store.findChatSession(ctx, req.SessionID)
		if err != nil {
			return chatSession{}, err
		}
		if sessionRecord.UserID != current.ID || sessionRecord.KnowledgeBaseID != kb.ID {
			return chatSession{}, sqlNotFound()
		}
		return sessionRecord, nil
	}
	id, err := randomToken()
	if err != nil {
		return chatSession{}, err
	}
	title := req.Message
	if len(title) > 80 {
		title = title[:80]
	}
	return a.store.createChatSession(ctx, chatSession{ID: id, UserID: current.ID, KnowledgeBaseID: kb.ID, Title: title})
}

func (a *app) runKnowledgeBaseAgent(ctx context.Context, current user, kb knowledgeBase, sessionID, message string, emit func(string, any)) (string, []retrievalEvidence, bool, error) {
	var evidence []retrievalEvidence
	retrievalCalled := false
	var retrievalErr error
	retrievalTool, err := functiontool.New(functiontool.Config{
		Name:        retrievalToolName,
		Description: "Searches the selected Knowledge Base. The backend enforces scope, permissions, limits, and citation metadata.",
	}, func(toolCtx agent.Context, args retrievalToolArgs) (retrievalToolResult, error) {
		retrievalCalled = true
		emit("retrieval", map[string]any{"query": args.Query})
		result, err := a.retrieveKnowledge(ctx, kb, args)
		if err != nil {
			retrievalErr = err
			return retrievalToolResult{}, err
		}
		evidence = append(evidence, result.Results...)
		return result, nil
	})
	if err != nil {
		return "", nil, false, err
	}

	modelInstance, err := a.chatModel(ctx)
	if err != nil {
		return "", nil, false, err
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "knowledge_base_assistant",
		Model:       modelInstance,
		Description: "Answers questions from a selected Knowledge Base.",
		Instruction: knowledgeBaseInstruction(kb),
		Tools:       []tool.Tool{retrievalTool},
	})
	if err != nil {
		return "", nil, false, err
	}
	sessionService := adksession.InMemoryService()
	runnr, err := runner.New(runner.Config{
		AppName:           chatAppName,
		Agent:             adkAgent,
		SessionService:    sessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		return "", nil, false, err
	}

	prompt, err := a.promptWithHistory(ctx, sessionID, message)
	if err != nil {
		return "", nil, false, err
	}
	userMessage := genai.NewContentFromText(prompt, genai.RoleUser)
	var answer strings.Builder
	sawPartial := false
	for event, err := range runnr.Run(ctx, strconv.FormatInt(current.ID, 10), sessionID, userMessage, agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			return "", evidence, retrievalCalled, err
		}
		if event == nil || event.Content == nil {
			continue
		}
		text := visibleText(event.Content)
		if text == "" {
			continue
		}
		if event.Partial {
			sawPartial = true
			answer.WriteString(text)
			emit("delta", map[string]any{"text": text})
			continue
		}
		if !sawPartial {
			answer.WriteString(text)
			emit("delta", map[string]any{"text": text})
		}
	}
	if retrievalErr != nil {
		return "", evidence, retrievalCalled, retrievalErr
	}
	return answer.String(), evidence, retrievalCalled, nil
}

func (a *app) retrieveKnowledge(ctx context.Context, kb knowledgeBase, args retrievalToolArgs) (retrievalToolResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return retrievalToolResult{}, errors.New("retrieval query is required")
	}
	if a.vectorIndex == nil {
		return retrievalToolResult{}, errors.New("vector index is unavailable")
	}
	limit := args.Limit
	if limit <= 0 || limit > maxRetrievalLimit {
		limit = maxRetrievalLimit
	}
	results, err := a.vectorIndex.search(ctx, query, vectorProbeLimit)
	if err != nil {
		return retrievalToolResult{}, err
	}
	out := retrievalToolResult{Results: make([]retrievalEvidence, 0, limit)}
	for _, result := range results {
		if len(out.Results) >= limit {
			break
		}
		chunk, err := a.store.findRetrievalChunk(ctx, result.ChunkID, kb.ID)
		if err != nil {
			if notFound(err) {
				continue
			}
			return retrievalToolResult{}, err
		}
		var anchor map[string]any
		_ = json.Unmarshal([]byte(chunk.SourceAnchorJSON), &anchor)
		out.Results = append(out.Results, retrievalEvidence{
			CitationID:   fmt.Sprintf("c%d", len(out.Results)+1),
			DocumentID:   chunk.DocumentID,
			DocumentName: chunk.DocumentName,
			ChunkID:      chunk.ChunkID,
			HeadingPath:  chunk.HeadingPath,
			SourceAnchor: anchor,
			Text:         chunk.Content,
		})
	}
	return out, nil
}

func (a *app) chatModel(ctx context.Context) (model.LLM, error) {
	setting, err := a.store.findProviderSetting(ctx, providerPurposeChat)
	if err != nil {
		return nil, err
	}
	if a.config.fakeProviders && strings.Contains(setting.BaseURL, "/fake-openai") {
		return fakeADKModel{name: setting.Model}, nil
	}
	return genaiopenai.New(genaiopenai.Config{
		APIKey:    setting.APIKey,
		BaseURL:   setting.BaseURL,
		ModelName: setting.Model,
		HTTPOptions: genaiopenai.HTTPOptions{
			Client: a.httpClient,
		},
	}), nil
}

func (a *app) promptWithHistory(ctx context.Context, sessionID, message string) (string, error) {
	messages, err := a.store.listChatMessages(ctx, sessionID, chatHistoryLimit)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Recent private conversation history:\n")
	for _, msg := range messages {
		if msg.Role == "tool" {
			continue
		}
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	b.WriteString("\nCurrent user question:\n")
	b.WriteString(message)
	return b.String(), nil
}

func knowledgeBaseInstruction(kb knowledgeBase) string {
	return "You answer questions for the selected Knowledge Base named " + kb.Name + ". Before any final answer, call retrieve_knowledge with a focused query. Use only retrieved evidence. If retrieval has no relevant results, say the documents do not contain enough information."
}

func unsupportedAnswerMessage() string {
	return "The selected Knowledge Base does not contain enough evidence to answer that question."
}

func (a *app) findPersistedCitation(ctx context.Context, sessionID, citationID string) (retrievalEvidence, bool, error) {
	messages, err := a.store.listChatMessages(ctx, sessionID, 100)
	if err != nil {
		return retrievalEvidence{}, false, err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		var metadata struct {
			Citations []retrievalEvidence `json:"citations"`
		}
		if err := json.Unmarshal([]byte(messages[i].Metadata), &metadata); err != nil {
			continue
		}
		for _, citation := range metadata.Citations {
			if citation.CitationID == citationID {
				return citation, true, nil
			}
		}
	}
	return retrievalEvidence{}, false, nil
}

func visibleText(content *genai.Content) string {
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil || part.Thought || part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func (a *app) registerChatCancel(sessionID string, cancel context.CancelFunc) {
	a.activeChatsMu.Lock()
	defer a.activeChatsMu.Unlock()
	a.activeChats[sessionID] = cancel
}

func (a *app) unregisterChatCancel(sessionID string) {
	a.activeChatsMu.Lock()
	defer a.activeChatsMu.Unlock()
	delete(a.activeChats, sessionID)
}

func sqlNotFound() error {
	return sql.ErrNoRows
}
