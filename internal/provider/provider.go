package provider

import "context"

type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

type ChatProvider interface {
	Stream(ctx context.Context, request ChatRequest, onToken func(string) error) error
}

type ChatRequest struct {
	Model    string
	Messages []Message
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StaticChatProvider struct{}

func (StaticChatProvider) Stream(ctx context.Context, request ChatRequest, onToken func(string) error) error {
	answer := "Based on the provided source, the office assistant can answer questions from indexed documents and return citations."
	for _, token := range splitWords(answer) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := onToken(token); err != nil {
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
