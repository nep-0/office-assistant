package chat

import (
	"encoding/json"

	"office-assistant/backend/domain"

	"github.com/nep-0/harness/agent"
)

func TranscriptFromMessages(messages []domain.ChatMessage) agent.Transcript {
	transcript := make(agent.Transcript, 0, len(messages))
	for _, message := range messages {
		role, ok := harnessRole(message.Role)
		if !ok {
			continue
		}
		converted := agent.Message{
			Role:       role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		if message.ToolCallsJSON != "" {
			_ = json.Unmarshal([]byte(message.ToolCallsJSON), &converted.ToolCalls)
		}
		transcript = append(transcript, converted)
	}
	return transcript
}

func PersistedMessages(sessionID string, transcript agent.Transcript, citations []RetrievalEvidence) ([]domain.ChatMessage, error) {
	messages := make([]domain.ChatMessage, 0, len(transcript))
	finalAssistant := -1
	for index, message := range transcript {
		if message.Role == agent.RoleAssistant && len(message.ToolCalls) == 0 {
			finalAssistant = index
		}
	}
	for index, message := range transcript {
		toolCalls, err := json.Marshal(message.ToolCalls)
		if err != nil {
			return nil, err
		}
		metadata := "{}"
		if index == finalAssistant {
			encoded, err := json.Marshal(map[string]any{"citations": citations})
			if err != nil {
				return nil, err
			}
			metadata = string(encoded)
		}
		messages = append(messages, domain.ChatMessage{
			SessionID:     sessionID,
			Role:          string(message.Role),
			Content:       message.Content,
			ToolCallsJSON: string(toolCalls),
			ToolCallID:    message.ToolCallID,
			Metadata:      metadata,
		})
	}
	return messages, nil
}

func WithoutFinalAssistant(transcript agent.Transcript) agent.Transcript {
	if len(transcript) == 0 {
		return transcript
	}
	last := transcript[len(transcript)-1]
	if last.Role == agent.RoleAssistant && len(last.ToolCalls) == 0 {
		return transcript[:len(transcript)-1]
	}
	return transcript
}

func IsVisibleMessage(message domain.ChatMessage) bool {
	if message.Role == "user" || message.Role == "error" {
		return true
	}
	if message.Role != "assistant" || message.Content == "" {
		return false
	}
	var calls []agent.ToolCall
	if err := json.Unmarshal([]byte(message.ToolCallsJSON), &calls); err != nil {
		return false
	}
	return len(calls) == 0
}

func harnessRole(role string) (agent.Role, bool) {
	switch agent.Role(role) {
	case agent.RoleSystem, agent.RoleDeveloper, agent.RoleUser, agent.RoleAssistant, agent.RoleTool:
		return agent.Role(role), true
	default:
		return "", false
	}
}
