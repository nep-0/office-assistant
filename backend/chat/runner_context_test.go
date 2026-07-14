package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"office-assistant/backend/domain"

	"github.com/nep-0/harness/agent"
)

func TestRunLimitsProviderContextWithoutSplittingToolGroups(t *testing.T) {
	historyTranscript := agent.Transcript{
		{Role: agent.RoleUser, Content: "question 1"},
		{Role: agent.RoleAssistant, Content: "answer 1"},
		{Role: agent.RoleUser, Content: "question 2"},
		{Role: agent.RoleAssistant, Content: "answer 2"},
		{Role: agent.RoleUser, Content: "question 3"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-3", Name: RetrievalToolName, Arguments: `{"query":"question 3"}`}}},
		{Role: agent.RoleTool, Content: `{"results":[]}`, ToolCallID: "call-3"},
		{Role: agent.RoleAssistant, Content: "answer 3"},
	}
	for turn := 4; turn <= 7; turn++ {
		historyTranscript = append(historyTranscript,
			agent.Message{Role: agent.RoleUser, Content: "question " + strconv.Itoa(turn)},
			agent.Message{Role: agent.RoleAssistant, Content: "answer " + strconv.Itoa(turn)},
		)
	}
	history, err := PersistedMessages("session-1", historyTranscript, nil)
	if err != nil {
		t.Fatal(err)
	}

	transport := &capturingChatTransport{}
	var debugPrompt agent.Transcript
	result, err := Run(context.Background(), RunRequest{
		Provider:     domain.ProviderSetting{BaseURL: "http://provider.test/v1", Model: "test-model"},
		HTTPClient:   &http.Client{Transport: transport},
		Instruction:  "developer instruction",
		History:      history,
		Message:      "current question",
		MaxTurns:     1,
		ContextTurns: 6,
		Retrieve: func(context.Context, RetrievalToolArgs) (RetrievalToolResult, error) {
			return RetrievalToolResult{}, nil
		},
		OnPrompt: func(prompt agent.Transcript) {
			debugPrompt = append(agent.Transcript(nil), prompt...)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	messages := transport.request.Messages
	if len(messages) == 0 || messages[0].Role != "developer" || messages[0].Content != "developer instruction" {
		t.Fatalf("leading instruction was not preserved: %#v", messages)
	}
	if messages[len(messages)-1].Role != "user" || messages[len(messages)-1].Content != "current question" {
		t.Fatalf("current turn was not preserved: %#v", messages)
	}
	var historicalQuestions []string
	callSeen := false
	resultSeen := false
	for _, message := range messages {
		if message.Role == "user" && message.Content != "current question" {
			historicalQuestions = append(historicalQuestions, message.Content)
		}
		if len(message.ToolCalls) == 1 && message.ToolCalls[0].ID == "call-3" {
			callSeen = true
		}
		if message.Role == "tool" && message.ToolCallID == "call-3" {
			resultSeen = true
		}
	}
	wantQuestions := []string{"question 3", "question 4", "question 5", "question 6", "question 7"}
	if !reflect.DeepEqual(historicalQuestions, wantQuestions) {
		t.Fatalf("historical questions = %#v, want %#v", historicalQuestions, wantQuestions)
	}
	if !callSeen || !resultSeen {
		t.Fatalf("tool group was split: call=%t result=%t messages=%#v", callSeen, resultSeen, messages)
	}
	if len(debugPrompt) != len(messages) {
		t.Fatalf("debug prompt has %d messages, provider received %d", len(debugPrompt), len(messages))
	}
	if len(result.Transcript) != len(historyTranscript)+3 {
		t.Fatalf("canonical transcript has %d messages, want %d", len(result.Transcript), len(historyTranscript)+3)
	}
}

type capturingChatTransport struct {
	request capturedChatRequest
}

type capturedChatRequest struct {
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
		ToolCalls  []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
	} `json:"messages"`
}

func (transport *capturingChatTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := json.NewDecoder(request.Body).Decode(&transport.request); err != nil {
		return nil, err
	}
	body := bytes.NewBufferString("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(body),
		Request:    request,
	}, nil
}
