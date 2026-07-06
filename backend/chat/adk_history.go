package chat

import (
	"context"

	"office-assistant/backend/domain"

	"google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func SeedADKSession(ctx context.Context, svc adksession.Service, appName, userID, sessionID, agentName string, messages []domain.ChatMessage) error {
	if _, err := svc.Get(ctx, &adksession.GetRequest{AppName: appName, UserID: userID, SessionID: sessionID}); err != nil {
		if _, createErr := svc.Create(ctx, &adksession.CreateRequest{AppName: appName, UserID: userID, SessionID: sessionID}); createErr != nil {
			return createErr
		}
	}
	found, err := svc.Get(ctx, &adksession.GetRequest{AppName: appName, UserID: userID, SessionID: sessionID})
	if err != nil {
		return err
	}
	for _, msg := range messages {
		content, author, ok := adkContentFromChatMessage(msg, agentName)
		if !ok {
			continue
		}
		event := adksession.NewEvent(ctx, "seed-history")
		event.Author = author
		if !msg.CreatedAt.IsZero() {
			event.Timestamp = msg.CreatedAt
		}
		event.LLMResponse = model.LLMResponse{Content: content}
		if err := svc.AppendEvent(ctx, found.Session, event); err != nil {
			return err
		}
	}
	return nil
}

func adkContentFromChatMessage(msg domain.ChatMessage, agentName string) (*genai.Content, string, bool) {
	if msg.Content == "" {
		return nil, "", false
	}
	switch msg.Role {
	case "user":
		return genai.NewContentFromText(msg.Content, genai.RoleUser), "user", true
	case "assistant":
		return genai.NewContentFromText(msg.Content, genai.RoleModel), agentName, true
	default:
		return nil, "", false
	}
}

func RenderStructuredHistoryForDebug(messages []domain.ChatMessage, currentMessage string) string {
	rendered := PromptWithHistory(messages, currentMessage)
	return rendered
}
