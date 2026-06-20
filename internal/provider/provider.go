package provider

import "context"

type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

type ChatProvider interface {
	Stream(ctx context.Context, request ChatRequest, onEvent func(StreamEvent) error) error
}

type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolSpec struct {
	Name        string
	Description string
}

type StreamEvent struct {
	Type     string
	Token    string
	ToolCall *ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type StaticChatProvider struct{}

func (StaticChatProvider) Stream(ctx context.Context, request ChatRequest, onEvent func(StreamEvent) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := onEvent(StreamEvent{
		Type: "tool_call",
		ToolCall: &ToolCall{
			ID:   "toolcall_static_retrieval",
			Name: "retrieve",
			Arguments: map[string]any{
				"query": "",
				"top_k": 5,
			},
		},
	}); err != nil {
		return err
	}

	answer := "Based on the provided source, the office assistant can answer questions from indexed documents and return citations."
	for _, token := range splitWords(answer) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := onEvent(StreamEvent{Type: "token", Token: token}); err != nil {
			return err
		}
	}
	return nil
}

type StaticEmbeddingProvider struct{}

func (StaticEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

func splitWords(value string) []string {
	words := []string{}
	current := ""
	for _, r := range value {
		current += string(r)
		if r == ' ' {
			words = append(words, current)
			current = ""
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}
