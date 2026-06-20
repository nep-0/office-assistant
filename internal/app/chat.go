package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"office-assistant/internal/provider"
	"office-assistant/internal/retrieval"
	"office-assistant/internal/storage"
	"office-assistant/internal/utils"
)

type ChatStore interface {
	GetProviderSettings(ctx context.Context) (storage.ProviderSettings, error)
	SaveChatMessage(ctx context.Context, sessionID, messageID, role, content string) error
	SaveCitation(ctx context.Context, citation storage.CitationRecord) error
}

type ChatService struct {
	store     ChatStore
	retrieval retrieval.Tool
}

type ChatInput struct {
	SessionID       string
	KnowledgeBaseID string
	Message         string
}

type ChatFinal struct {
	SessionID  string
	MessageID  string
	ElapsedMS  int64
	Answer     string
	Citations  []Citation
	Provider   map[string]string
	SourceType string
	ToolCalls  []ToolCall
}

type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Citation struct {
	ID             string  `json:"id"`
	DocumentID     string  `json:"document_id"`
	ChunkID        string  `json:"chunk_id"`
	SourceFileName string  `json:"source_file_name"`
	PageNumber     int     `json:"page_number"`
	Preview        string  `json:"preview"`
	Score          float64 `json:"score"`
}

func NewChatService(store ChatStore, retrievalTool retrieval.Tool) *ChatService {
	if retrievalTool == nil {
		retrievalTool = retrieval.StaticTool{}
	}
	return &ChatService{store: store, retrieval: retrievalTool}
}

func (s *ChatService) Stream(ctx context.Context, input ChatInput, onToken func(string) error) (ChatFinal, error) {
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		return ChatFinal{}, errors.New("message is required")
	}
	if input.SessionID == "" {
		input.SessionID = utils.NewID("session")
	}
	if input.KnowledgeBaseID == "" {
		input.KnowledgeBaseID = "demo"
	}

	userMessageID := utils.NewID("msg")
	assistantMessageID := utils.NewID("msg")
	if err := s.store.SaveChatMessage(ctx, input.SessionID, userMessageID, "user", input.Message); err != nil {
		return ChatFinal{}, err
	}

	settings, err := s.store.GetProviderSettings(ctx)
	if err != nil {
		return ChatFinal{}, err
	}

	chatProvider := buildChatProvider(settings.Chat)
	answer := strings.Builder{}
	citations := []Citation{}
	toolCalls := []ToolCall{}
	start := time.Now()
	err = chatProvider.Stream(ctx, provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "system", Content: "Use the retrieve tool when document evidence is needed. If you do not have enough retrieved context, say so. Keep the answer concise and cite supporting sources after retrieval."},
			{Role: "user", Content: input.Message},
		},
		Tools: []provider.ToolSpec{
			{Name: "retrieve", Description: "Retrieve relevant chunks from the selected knowledge base."},
		},
	}, func(event provider.StreamEvent) error {
		switch event.Type {
		case "token":
			answer.WriteString(event.Token)
			return onToken(event.Token)
		case "tool_call":
			if event.ToolCall == nil || event.ToolCall.Name != "retrieve" {
				return nil
			}
			toolCalls = append(toolCalls, ToolCall{ID: event.ToolCall.ID, Name: event.ToolCall.Name})
			retrieved, err := s.retrieval.Retrieve(ctx, retrieval.Input{
				KnowledgeBaseID: input.KnowledgeBaseID,
				Query:           toolQuery(event.ToolCall, input.Message),
				TopK:            toolTopK(event.ToolCall, 5),
			})
			if err != nil {
				return err
			}
			for _, chunk := range retrieved.Chunks {
				citationID := utils.NewID("cit")
				citations = append(citations, citationFromChunk(chunk, citationID))
			}
			return nil
		default:
			return nil
		}
	})
	if err != nil {
		return ChatFinal{}, err
	}

	if err := s.store.SaveChatMessage(ctx, input.SessionID, assistantMessageID, "assistant", answer.String()); err != nil {
		return ChatFinal{}, err
	}

	for _, citation := range citations {
		pageNumber := citation.PageNumber
		score := citation.Score
		record := storage.CitationRecord{
			ID:             citation.ID,
			MessageID:      assistantMessageID,
			DocumentID:     citation.DocumentID,
			ChunkID:        citation.ChunkID,
			SourceFileName: citation.SourceFileName,
			PageNumber:     &pageNumber,
			Preview:        citation.Preview,
			Score:          &score,
		}
		if err := s.store.SaveCitation(ctx, record); err != nil {
			return ChatFinal{}, err
		}
	}

	return ChatFinal{
		SessionID:  input.SessionID,
		MessageID:  assistantMessageID,
		ElapsedMS:  time.Since(start).Milliseconds(),
		Answer:     answer.String(),
		Citations:  citations,
		Provider:   providerSummary(settings.Chat),
		SourceType: sourceType(citations),
		ToolCalls:  toolCalls,
	}, nil
}

func toolQuery(call *provider.ToolCall, fallback string) string {
	if call == nil {
		return fallback
	}
	if value, ok := call.Arguments["query"].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func toolTopK(call *provider.ToolCall, fallback int) int {
	if call == nil {
		return fallback
	}
	switch value := call.Arguments["top_k"].(type) {
	case int:
		if value > 0 {
			return value
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return fallback
}

func sourceType(citations []Citation) string {
	if len(citations) == 0 {
		return "none"
	}
	return "retrieval-tool"
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

func citationFromChunk(c retrieval.Chunk, id string) Citation {
	return Citation{
		ID:             id,
		DocumentID:     c.DocumentID,
		ChunkID:        c.ChunkID,
		SourceFileName: c.SourceFileName,
		PageNumber:     c.PageNumber,
		Preview:        c.Preview,
		Score:          c.Score,
	}
}
