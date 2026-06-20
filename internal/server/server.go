package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"office-assistant/internal/app"
	"office-assistant/internal/auth"
	"office-assistant/internal/document"
	"office-assistant/internal/retrieval"
	"office-assistant/internal/storage"
)

type Store interface {
	Ping(ctx context.Context) error
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, user storage.User) (storage.User, error)
	GetUserByUsername(ctx context.Context, username string) (storage.User, error)
	GetUserByID(ctx context.Context, id string) (storage.User, error)
	ListUsers(ctx context.Context) ([]storage.User, error)
	CreateKnowledgeBase(ctx context.Context, kb storage.KnowledgeBase) (storage.KnowledgeBase, error)
	AddKnowledgeBaseMember(ctx context.Context, knowledgeBaseID, userID string) error
	CanAccessKnowledgeBase(ctx context.Context, user storage.User, knowledgeBaseID string) (bool, error)
	ListKnowledgeBases(ctx context.Context, user storage.User) ([]storage.KnowledgeBase, error)
	CreateDocument(ctx context.Context, doc storage.Document) (storage.Document, error)
	ListDocuments(ctx context.Context, knowledgeBaseID string) ([]storage.Document, error)
	UpdateDocumentStatus(ctx context.Context, documentID, status, reason string) error
	ReplaceDocumentChunks(ctx context.Context, documentID string, chunks []storage.Chunk) error
	GetProviderSettings(ctx context.Context) (storage.ProviderSettings, error)
	SaveProviderSettings(ctx context.Context, settings storage.ProviderSettings) error
	SaveChatMessage(ctx context.Context, sessionID, messageID, role, content string) error
	SaveCitation(ctx context.Context, citation storage.CitationRecord) error
}

type Options struct {
	Store         Store
	Logger        *slog.Logger
	JWTSecret     string
	UploadDir     string
	MarkItDownURL string
	Processor     document.Processor
	Retrieval     retrieval.Tool
}

type api struct {
	store     Store
	logger    *slog.Logger
	tokens    *auth.Manager
	accounts  *app.AccountService
	documents *app.DocumentService
	chat      *app.ChatService
}

func New(options Options) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}

	tokens := auth.NewManager(options.JWTSecret, 24*time.Hour)
	api := &api{
		store:  options.Store,
		logger: logger,
		tokens: tokens,
	}
	api.accounts = app.NewAccountService(options.Store, tokens)
	api.documents = app.NewDocumentService(app.DocumentServiceOptions{
		Store:     options.Store,
		Processor: processorOrDefault(options),
		UploadDir: uploadDirOrDefault(options.UploadDir),
		Logger:    logger,
	})
	api.chat = app.NewChatService(options.Store, retrievalOrDefault(options))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.health)
	mux.HandleFunc("POST /api/auth/bootstrap", api.bootstrapAdmin)
	mux.HandleFunc("POST /api/auth/login", api.login)
	mux.Handle("GET /api/me", api.requireAuth(http.HandlerFunc(api.me)))
	mux.Handle("GET /api/provider-settings", api.requireAdmin(http.HandlerFunc(api.getProviderSettings)))
	mux.Handle("PUT /api/provider-settings", api.requireAdmin(http.HandlerFunc(api.putProviderSettings)))
	mux.Handle("GET /api/admin/users", api.requireAdmin(http.HandlerFunc(api.listUsers)))
	mux.Handle("POST /api/admin/users", api.requireAdmin(http.HandlerFunc(api.createUser)))
	mux.Handle("GET /api/knowledge-bases", api.requireAuth(http.HandlerFunc(api.listKnowledgeBases)))
	mux.Handle("POST /api/knowledge-bases", api.requireAdmin(http.HandlerFunc(api.createKnowledgeBase)))
	mux.Handle("POST /api/knowledge-bases/{id}/members", api.requireAdmin(http.HandlerFunc(api.addKnowledgeBaseMember)))
	mux.Handle("GET /api/knowledge-bases/{id}/documents", api.requireAuth(http.HandlerFunc(api.listDocuments)))
	mux.Handle("POST /api/knowledge-bases/{id}/documents", api.requireAuth(http.HandlerFunc(api.uploadDocument)))
	mux.Handle("POST /api/chat/stream", api.requireAuth(http.HandlerFunc(api.streamChat)))
	return logRequests(logger, mux)
}

func uploadDirOrDefault(uploadDir string) string {
	if uploadDir == "" {
		return "data/uploads"
	}
	return uploadDir
}

func processorOrDefault(options Options) document.Processor {
	if options.Processor != nil {
		return options.Processor
	}
	return document.HTTPProcessor{BaseURL: options.MarkItDownURL}
}

func retrievalOrDefault(options Options) retrieval.Tool {
	if options.Retrieval != nil {
		return options.Retrieval
	}
	return retrieval.StaticTool{}
}
