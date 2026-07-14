package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func FakeChatHTTPClient() *http.Client {
	return &http.Client{Transport: fakeChatTransport{}}
}

type fakeChatTransport struct{}

type fakeChatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
}

func (fakeChatTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload fakeChatRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var stream bytes.Buffer
	if strings.Contains(payload.Model, "no-retrieval") {
		writeFakeTextStream(&stream, payload.Model, "Ungrounded answer.")
	} else if toolResult(payload.Messages) == "" {
		arguments, _ := json.Marshal(map[string]any{"query": latestFakeUserText(payload.Messages), "limit": 5})
		delta := map[string]any{
			"role": "assistant",
			"tool_calls": []map[string]any{{
				"index": 0,
				"id":    "fake-retrieval-call",
				"type":  "function",
				"function": map[string]any{
					"name":      "retrieve_knowledge",
					"arguments": string(arguments),
				},
			}},
		}
		writeFakeChunk(&stream, payload.Model, delta, "tool_calls")
		stream.WriteString("data: [DONE]\n\n")
	} else {
		text := "Based on the retrieved Knowledge Base evidence, the documents contain relevant information. [c1]"
		if strings.Contains(toolResult(payload.Messages), `"results":[]`) {
			text = "The documents do not contain enough information."
		}
		writeFakeTextStream(&stream, payload.Model, text)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader(stream.Bytes())),
		Request:    request,
	}, nil
}

func writeFakeTextStream(stream *bytes.Buffer, model, text string) {
	for index, word := range strings.Fields(text) {
		if index > 0 {
			word = " " + word
		}
		writeFakeChunk(stream, model, map[string]any{"role": "assistant", "content": word}, "")
	}
	writeFakeChunk(stream, model, map[string]any{}, "stop")
	stream.WriteString("data: [DONE]\n\n")
}

func writeFakeChunk(stream *bytes.Buffer, model string, delta map[string]any, finishReason string) {
	chunk := map[string]any{
		"id":      "fake-chat",
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finishReason,
		}},
	}
	encoded, _ := json.Marshal(chunk)
	fmt.Fprintf(stream, "data: %s\n\n", encoded)
}

func latestFakeUserText(messages []struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return messages[index].Content
		}
	}
	return "knowledge base question"
}

func toolResult(messages []struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "tool" {
			return messages[index].Content
		}
		if messages[index].Role == "user" {
			return ""
		}
	}
	return ""
}
