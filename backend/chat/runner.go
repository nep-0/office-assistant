package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"office-assistant/backend/domain"

	"github.com/nep-0/harness/agent"
	"github.com/nep-0/harness/middleware"
)

const RetrievalToolName = "retrieve_knowledge"

var retrievalToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Focused search query proposed by the model."
    },
    "limit": {
      "type": "integer",
      "description": "Optional desired result count. The backend enforces a maximum.",
      "minimum": 1
    }
  },
  "required": ["query"],
  "additionalProperties": false
}`)

type RunRequest struct {
	Provider     domain.ProviderSetting
	HTTPClient   *http.Client
	Instruction  string
	History      []domain.ChatMessage
	Message      string
	MaxTurns     int
	ContextTurns int
	Retrieve     func(context.Context, RetrievalToolArgs) (RetrievalToolResult, error)
	OnTextDelta  func(string)
	OnRetrieval  func(RetrievalToolArgs)
	OnPrompt     func(agent.Transcript)
	OnGeneration func(time.Duration)
}

type RunResult struct {
	Transcript      agent.Transcript
	NewMessages     agent.Transcript
	Answer          string
	Evidence        []RetrievalEvidence
	RetrievalCalled bool
}

func Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if req.Retrieve == nil {
		return RunResult{}, errors.New("chat: retrieval handler is required")
	}
	window, err := middleware.NewSlidingWindow(req.ContextTurns)
	if err != nil {
		return RunResult{}, err
	}
	base := TranscriptFromMessages(req.History)
	modelTranscript := make(agent.Transcript, 0, len(base)+1)
	modelTranscript = append(modelTranscript, agent.Message{Role: agent.RoleDeveloper, Content: req.Instruction})
	modelTranscript = append(modelTranscript, base...)
	if req.OnPrompt != nil {
		prompt := append(agent.Transcript(nil), modelTranscript...)
		prompt = append(prompt, agent.Message{Role: agent.RoleUser, Content: req.Message})
		prompt, err = window.Context(ctx, prompt)
		if err != nil {
			return RunResult{}, err
		}
		req.OnPrompt(prompt)
	}

	var stateMu sync.Mutex
	var callbackMu sync.Mutex
	var evidence []RetrievalEvidence
	retrievalCalled := false
	var retrievalErr error
	tool := agent.Tool{
		Name:        RetrievalToolName,
		Description: "Searches the selected Knowledge Base. The backend enforces scope, permissions, limits, and citation metadata.",
		Parameters:  retrievalToolSchema,
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args RetrievalToolArgs
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			stateMu.Lock()
			retrievalCalled = true
			stateMu.Unlock()
			if req.OnRetrieval != nil {
				callbackMu.Lock()
				req.OnRetrieval(args)
				callbackMu.Unlock()
			}
			result, err := req.Retrieve(ctx, args)
			if err != nil {
				stateMu.Lock()
				if retrievalErr == nil {
					retrievalErr = err
				}
				stateMu.Unlock()
				return "", err
			}
			stateMu.Lock()
			for index := range result.Results {
				result.Results[index].CitationID = fmt.Sprintf("c%d", len(evidence)+index+1)
			}
			evidence = append(evidence, result.Results...)
			stateMu.Unlock()
			encoded, err := json.Marshal(result)
			return string(encoded), err
		},
	}

	runner, err := agent.NewRunner(
		agent.WithAPIKey(req.Provider.APIKey),
		agent.WithBaseURL(req.Provider.BaseURL),
		agent.WithModel(req.Provider.Model),
		agent.WithHTTPClient(req.HTTPClient),
		agent.WithMaxTurns(req.MaxTurns),
		agent.WithMiddleware(window),
		agent.WithTool(tool),
		agent.WithEventHandler(func(event agent.Event) error {
			if event.Type == agent.EventTextDelta && req.OnTextDelta != nil {
				req.OnTextDelta(event.Delta)
			}
			return nil
		}),
	)
	if err != nil {
		return RunResult{}, err
	}

	started := time.Now()
	snapshot, runErr := runner.RunTurn(ctx, agent.RunSnapshot{Transcript: modelTranscript}, []agent.Message{{Role: agent.RoleUser, Content: req.Message}})
	if req.OnGeneration != nil {
		req.OnGeneration(time.Since(started))
	}
	newStart := len(modelTranscript)
	if newStart > len(snapshot.Transcript) {
		newStart = len(snapshot.Transcript)
	}
	newMessages := append(agent.Transcript(nil), snapshot.Transcript[newStart:]...)
	result := RunResult{
		Transcript:  snapshot.Transcript,
		NewMessages: newMessages,
	}
	stateMu.Lock()
	result.Evidence = append([]RetrievalEvidence(nil), evidence...)
	result.RetrievalCalled = retrievalCalled
	capturedRetrievalErr := retrievalErr
	stateMu.Unlock()
	for i := len(newMessages) - 1; i >= 0; i-- {
		if newMessages[i].Role == agent.RoleAssistant && len(newMessages[i].ToolCalls) == 0 {
			result.Answer = newMessages[i].Content
			break
		}
	}
	if runErr != nil {
		return result, runErr
	}
	if capturedRetrievalErr != nil {
		return result, capturedRetrievalErr
	}
	return result, nil
}
