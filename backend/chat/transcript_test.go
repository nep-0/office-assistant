package chat

import (
	"testing"

	"office-assistant/backend/domain"

	"github.com/nep-0/harness/agent"
)

func TestTranscriptPersistenceRoundTrip(t *testing.T) {
	original := agent.Transcript{
		{Role: agent.RoleUser, Content: "question"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-1", Name: RetrievalToolName, Arguments: `{"query":"policy"}`}}},
		{Role: agent.RoleTool, Content: `{"results":[]}`, ToolCallID: "call-1"},
		{Role: agent.RoleAssistant, Content: "answer"},
	}
	persisted, err := PersistedMessages("session-1", original, []RetrievalEvidence{{CitationID: "c1", DocumentID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 4 || persisted[1].ToolCallsJSON == "[]" || persisted[2].ToolCallID != "call-1" {
		t.Fatalf("persisted=%#v", persisted)
	}
	roundTrip := TranscriptFromMessages(persisted)
	if len(roundTrip) != len(original) || roundTrip[1].ToolCalls[0].Arguments != `{"query":"policy"}` || roundTrip[2].ToolCallID != "call-1" {
		t.Fatalf("roundTrip=%#v", roundTrip)
	}
	if !IsVisibleMessage(persisted[0]) || IsVisibleMessage(persisted[1]) || IsVisibleMessage(persisted[2]) || !IsVisibleMessage(persisted[3]) {
		t.Fatalf("unexpected visibility for %#v", persisted)
	}
}

func TestTranscriptSkipsApplicationErrorRows(t *testing.T) {
	transcript := TranscriptFromMessages([]domain.ChatMessage{{Role: "error", Content: "failed"}})
	if len(transcript) != 0 {
		t.Fatalf("transcript=%#v", transcript)
	}
}
