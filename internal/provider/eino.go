package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	einoembeddingopenai "github.com/cloudwego/eino-ext/components/embedding/openai"
	einomodelopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

type EinoOpenAICompatible struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

func (p EinoOpenAICompatible) Embed(ctx context.Context, model string, text string) ([]float64, error) {
	embedder, err := einoembeddingopenai.NewEmbedder(ctx, &einoembeddingopenai.EmbeddingConfig{
		APIKey:  apiKeyOrPlaceholder(p.APIKey),
		BaseURL: p.BaseURL,
		Model:   model,
		Timeout: p.timeout(),
	})
	if err != nil {
		return nil, fmt.Errorf("create eino embedder: %w", err)
	}

	vectors, err := embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed strings with eino: %w", err)
	}
	if len(vectors) == 0 {
		return nil, errors.New("embedding response contained no vectors")
	}
	return vectors[0], nil
}

func (p EinoOpenAICompatible) StreamChat(ctx context.Context, request ChatRequest, onEvent func(StreamEvent) error) error {
	chatModel, err := einomodelopenai.NewChatModel(ctx, &einomodelopenai.ChatModelConfig{
		APIKey:  apiKeyOrPlaceholder(p.APIKey),
		BaseURL: p.BaseURL,
		Model:   request.Model,
		Timeout: p.timeout(),
	})
	if err != nil {
		return fmt.Errorf("create eino chat model: %w", err)
	}

	messages := make([]*schema.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, toEinoMessage(message))
	}

	stream, err := chatModel.Stream(ctx, messages)
	if err != nil {
		return fmt.Errorf("start eino chat stream: %w", err)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("receive eino chat stream: %w", err)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		if err := onEvent(StreamEvent{Type: "token", Token: chunk.Content}); err != nil {
			return err
		}
	}

	return nil
}

func (p EinoOpenAICompatible) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 60 * time.Second
}

func toEinoMessage(message Message) *schema.Message {
	switch message.Role {
	case "system":
		return schema.SystemMessage(message.Content)
	case "assistant":
		return schema.AssistantMessage(message.Content, nil)
	default:
		return schema.UserMessage(message.Content)
	}
}

func apiKeyOrPlaceholder(apiKey string) string {
	if apiKey == "" {
		return "unused"
	}
	return apiKey
}

type OpenAIEmbeddingProvider struct {
	Client EinoOpenAICompatible
	Model  string
}

func (p OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	return p.Client.Embed(ctx, p.Model, text)
}

type OpenAIChatProvider struct {
	Client EinoOpenAICompatible
	Model  string
}

func (p OpenAIChatProvider) Stream(ctx context.Context, request ChatRequest, onEvent func(StreamEvent) error) error {
	request.Model = p.Model
	return p.Client.StreamChat(ctx, request, onEvent)
}
