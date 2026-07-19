package chat

import (
	"context"
	"testing"

	"github.com/nep-0/harness/agent"
)

func TestMergeInstructionsProducesOneLeadingInstruction(t *testing.T) {
	transcript := agent.Transcript{
		{Role: agent.RoleSystem, Content: "base"},
		{Role: agent.RoleDeveloper, Content: "stable metadata"},
		{Role: agent.RoleUser, Content: "question"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-1", Name: RetrievalToolName}}},
		{Role: agent.RoleTool, Content: "result", ToolCallID: "call-1"},
		{Role: agent.RoleDeveloper, Content: "volatile metadata"},
	}

	merged, err := (MergeInstructions{}).Context(context.Background(), transcript)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 4 {
		t.Fatalf("merged transcript = %#v", merged)
	}
	if merged[0].Role != agent.RoleSystem || merged[0].Content != "base\n\nstable metadata\n\nvolatile metadata" {
		t.Fatalf("merged instruction = %#v", merged[0])
	}
	for _, message := range merged[1:] {
		if message.Role == agent.RoleSystem || message.Role == agent.RoleDeveloper {
			t.Fatalf("unexpected additional instruction: %#v", message)
		}
	}
	if merged[2].ToolCalls[0].ID != "call-1" || merged[3].ToolCallID != "call-1" {
		t.Fatalf("tool group changed: %#v", merged)
	}
}
