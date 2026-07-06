package chat

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"office-assistant/backend/domain"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

type Emitter func(event string, payload any)

type Runner struct {
	AppName           string
	RetrievalToolName string
	MaxRetrievalLimit int
}

const knowledgeBaseAgentName = "knowledge_base_assistant"

type RunRequest struct {
	Current   domain.User
	KB        domain.KnowledgeBase
	SessionID string
	Message   string
}

type RunDeps struct {
	LoadHistory      func(context.Context, string) ([]domain.ChatMessage, error)
	Retrieve         func(context.Context, domain.KnowledgeBase, RetrievalToolArgs) (RetrievalToolResult, error)
	Model            func(context.Context) (model.LLM, error)
	RecordPrompt     func(context.Context, domain.KnowledgeBase, string, string)
	RecordGeneration func(context.Context, domain.KnowledgeBase, string, time.Duration)
	CorrelationID    func(context.Context) string
}

type RunResult struct {
	Answer          string
	Evidence        []RetrievalEvidence
	RetrievalCalled bool
}

func (r Runner) Run(ctx context.Context, req RunRequest, deps RunDeps, emit Emitter) (RunResult, error) {
	generationStarted := time.Now()
	var evidence []RetrievalEvidence
	retrievalCalled := false
	var retrievalErr error

	retrievalTool, err := functiontool.New(functiontool.Config{
		Name:        r.RetrievalToolName,
		Description: "Searches the selected Knowledge Base. The backend enforces scope, permissions, limits, and citation metadata.",
	}, func(toolCtx agent.Context, args RetrievalToolArgs) (RetrievalToolResult, error) {
		retrievalCalled = true
		emit("retrieval", map[string]any{"query": args.Query})
		result, err := deps.Retrieve(ctx, req.KB, args)
		if err != nil {
			retrievalErr = err
			return RetrievalToolResult{}, err
		}
		evidence = append(evidence, result.Results...)
		return result, nil
	})
	if err != nil {
		return RunResult{}, err
	}

	modelInstance, err := deps.Model(ctx)
	if err != nil {
		return RunResult{}, err
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        knowledgeBaseAgentName,
		Model:       modelInstance,
		Description: "Answers questions from a selected Knowledge Base.",
		Instruction: KnowledgeBaseInstruction(req.KB),
		Tools:       []tool.Tool{retrievalTool},
	})
	if err != nil {
		return RunResult{}, err
	}
	sessionService := adksession.InMemoryService()
	runnr, err := runner.New(runner.Config{
		AppName:           r.AppName,
		Agent:             adkAgent,
		SessionService:    sessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		return RunResult{}, err
	}

	history, err := deps.LoadHistory(ctx, req.SessionID)
	if err != nil {
		return RunResult{}, err
	}
	userID := strconv.FormatInt(req.Current.ID, 10)
	if err := SeedADKSession(ctx, sessionService, r.AppName, userID, req.SessionID, knowledgeBaseAgentName, history); err != nil {
		return RunResult{}, err
	}
	if deps.RecordPrompt != nil {
		deps.RecordPrompt(ctx, req.KB, req.SessionID, RenderStructuredHistoryForDebug(history, req.Message))
	}

	userMessage := genai.NewContentFromText(req.Message, genai.RoleUser)
	var answer strings.Builder
	sawPartial := false
	correlationID := ""
	if deps.CorrelationID != nil {
		correlationID = deps.CorrelationID(ctx)
	}
	log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_started", correlationID, req.SessionID)
	for event, err := range runnr.Run(ctx, userID, req.SessionID, userMessage, agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_error error=%q", correlationID, req.SessionID, err.Error())
			return RunResult{Evidence: evidence, RetrievalCalled: retrievalCalled}, err
		}
		if event == nil || event.Content == nil {
			continue
		}
		text := VisibleText(event.Content)
		if text == "" {
			continue
		}
		if event.Partial {
			sawPartial = true
			answer.WriteString(text)
			emit("delta", map[string]any{"text": text})
			continue
		}
		if !sawPartial {
			answer.WriteString(text)
			emit("delta", map[string]any{"text": text})
		}
	}
	if retrievalErr != nil {
		return RunResult{Evidence: evidence, RetrievalCalled: retrievalCalled}, retrievalErr
	}
	duration := time.Since(generationStarted)
	if deps.RecordGeneration != nil {
		deps.RecordGeneration(ctx, req.KB, req.SessionID, duration)
	}
	log.Printf("correlation_id=%s provider=chat session_id=%s event=provider_call_completed duration_ms=%d", correlationID, req.SessionID, duration.Milliseconds())
	return RunResult{Answer: answer.String(), Evidence: evidence, RetrievalCalled: retrievalCalled}, nil
}
