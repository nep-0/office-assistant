package chat

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/nep-0/harness/agent"
)

// MergeInstructions collapses all system and developer messages into one
// leading instruction for providers that accept only one such message.
type MergeInstructions struct{}

func (MergeInstructions) ID() string                             { return "merge_instructions" }
func (MergeInstructions) MarshalState() (json.RawMessage, error) { return nil, nil }
func (MergeInstructions) UnmarshalState(json.RawMessage) error   { return nil }

func (MergeInstructions) Context(_ context.Context, transcript agent.Transcript) (agent.Transcript, error) {
	role := agent.RoleDeveloper
	contents := make([]string, 0, 3)
	contextTranscript := make(agent.Transcript, 0, len(transcript))
	for _, message := range transcript {
		if message.Role == agent.RoleSystem || message.Role == agent.RoleDeveloper {
			if len(contents) == 0 {
				role = message.Role
			}
			if content := strings.TrimSpace(message.Content); content != "" {
				contents = append(contents, content)
			}
			continue
		}
		contextTranscript = append(contextTranscript, message)
	}
	if len(contents) == 0 {
		return contextTranscript, nil
	}
	merged := make(agent.Transcript, 0, len(contextTranscript)+1)
	merged = append(merged, agent.Message{Role: role, Content: strings.Join(contents, "\n\n")})
	merged = append(merged, contextTranscript...)
	return merged, nil
}
