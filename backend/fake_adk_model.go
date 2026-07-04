package main

import (
	"context"
	"iter"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

type fakeADKModel struct {
	name string
}

func (m fakeADKModel) Name() string {
	if m.name == "" {
		return "fake-chat"
	}
	return m.name
}

func (m fakeADKModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if ctx.Err() != nil {
			yield(nil, ctx.Err())
			return
		}
		if strings.Contains(m.Name(), "no-retrieval") {
			yield(&model.LLMResponse{Content: genai.NewContentFromText("Ungrounded answer.", genai.RoleModel)}, nil)
			return
		}
		if !hasFunctionResponse(req.Contents, retrievalToolName) {
			yield(&model.LLMResponse{
				Content: &genai.Content{
					Role: genai.RoleModel,
					Parts: []*genai.Part{
						genai.NewPartFromFunctionCall(retrievalToolName, map[string]any{
							"query": latestUserText(req.Contents),
							"limit": maxRetrievalLimit,
						}),
					},
				},
			}, nil)
			return
		}
		text := "Based on the retrieved Knowledge Base evidence, " + summarizeFunctionResponse(req.Contents)
		if !stream {
			yield(&model.LLMResponse{Content: genai.NewContentFromText(text, genai.RoleModel)}, nil)
			return
		}
		for _, piece := range splitFakeStream(text) {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			if !yield(&model.LLMResponse{
				Content:      genai.NewContentFromText(piece, genai.RoleModel),
				Partial:      true,
				TurnComplete: false,
			}, nil) {
				return
			}
		}
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText(text, genai.RoleModel),
			TurnComplete: true,
		}, nil)
	}
}

func hasFunctionResponse(contents []*genai.Content, name string) bool {
	for _, content := range contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == name {
				return true
			}
		}
	}
	return false
}

func latestUserText(contents []*genai.Content) string {
	for i := len(contents) - 1; i >= 0; i-- {
		if contents[i].Role != genai.RoleUser {
			continue
		}
		var b strings.Builder
		for _, part := range contents[i].Parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return "knowledge base question"
}

func summarizeFunctionResponse(contents []*genai.Content) string {
	for i := len(contents) - 1; i >= 0; i-- {
		for _, part := range contents[i].Parts {
			if part.FunctionResponse == nil || part.FunctionResponse.Name != retrievalToolName {
				continue
			}
			if results, ok := part.FunctionResponse.Response["results"].([]any); ok && len(results) == 0 {
				return "the documents do not contain enough information."
			}
			return "the documents contain relevant information. [c1]"
		}
	}
	return "the documents do not contain enough information."
}

func splitFakeStream(text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	out := make([]string, 0, len(words))
	for i, word := range words {
		if i > 0 {
			word = " " + word
		}
		out = append(out, word)
	}
	return out
}
