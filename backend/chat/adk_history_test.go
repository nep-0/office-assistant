package chat

import (
	"context"
	"testing"

	"office-assistant/backend/domain"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestSeedADKSessionPreservesMessageRoles(t *testing.T) {
	ctx := context.Background()
	service := adksession.InMemoryService()
	messages := []domain.ChatMessage{
		{Role: "user", Content: "What is the policy?"},
		{Role: "assistant", Content: "Use retrieved evidence."},
		{Role: "tool", Content: "internal"},
		{Role: "error", Content: "internal error"},
	}

	err := SeedADKSession(ctx, service, "app", "user-1", "session-1", "agent", messages)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	found, err := service.Get(ctx, &adksession.GetRequest{AppName: "app", UserID: "user-1", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	events := found.Session.Events()
	if events.Len() != 2 {
		t.Fatalf("expected 2 visible events, got %d", events.Len())
	}
	if got := events.At(0).Content.Role; got != genai.RoleUser {
		t.Fatalf("expected user role for first event, got %q", got)
	}
	if got := events.At(0).Author; got != "user" {
		t.Fatalf("expected user author for first event, got %q", got)
	}
	if got := events.At(1).Content.Role; got != genai.RoleModel {
		t.Fatalf("expected model role for second event, got %q", got)
	}
	if got := events.At(1).Author; got != "agent" {
		t.Fatalf("expected agent author for second event, got %q", got)
	}
}
