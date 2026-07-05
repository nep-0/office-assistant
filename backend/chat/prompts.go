package chat

import (
	"strings"

	"office-assistant/backend/domain"

	"google.golang.org/genai"
)

func PromptWithHistory(messages []domain.ChatMessage, message string) string {
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
	return b.String()
}

func KnowledgeBaseInstruction(kb domain.KnowledgeBase) string {
	return "You answer questions for the selected Knowledge Base named " + kb.Name + ". Before any final answer, call retrieve_knowledge with a focused query. Use only retrieved evidence. If retrieval has no relevant results, say the documents do not contain enough information."
}

func UnsupportedAnswerMessage() string {
	return "The selected Knowledge Base does not contain enough evidence to answer that question."
}

func VisibleText(content *genai.Content) string {
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil || part.Thought || part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return b.String()
}
