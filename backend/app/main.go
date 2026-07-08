package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	chatpkg "office-assistant/backend/chat"
	docpkg "office-assistant/backend/documents"
	"office-assistant/backend/domain"
	"office-assistant/backend/httpapi"
	ingestionpkg "office-assistant/backend/ingestion"
	"office-assistant/backend/search"
	storepkg "office-assistant/backend/store"
	"office-assistant/backend/utils"
)

type app struct {
	startedAt        time.Time
	config           config
	store            *storepkg.Store
	httpClient       *http.Client
	chatHTTPClient   *http.Client
	chunkingStrategy ingestionpkg.ChunkingStrategy
	vectorIndex      *search.VectorIndex
	activeChats      *chatpkg.CancelRegistry
}

type config struct {
	addr               string
	databasePath       string
	storageRoot        string
	documentURL        string
	ocrURL             string
	fakeProviders      bool
	debugEnvEnabled    bool
	chatRequestTimeout time.Duration
	defaultProviders   map[string]domain.ProviderSetting
}

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	StartedAt string `json:"started_at"`
}

type readinessResponse struct {
	Status       string                      `json:"status"`
	Dependencies map[string]dependencyStatus `json:"dependencies"`
}

type dependencyStatus struct {
	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Message string `json:"message,omitempty"`
}

func Run() {
	cfg := loadConfig()
	store, err := openStore(cfg.databasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureProviderDefaults(context.Background(), cfg.defaultProviders); err != nil {
		log.Fatal(err)
	}
	a := &app{
		startedAt:        time.Now().UTC(),
		config:           cfg,
		store:            store,
		chunkingStrategy: ingestionpkg.MarkdownChunkingStrategy{},
		activeChats:      chatpkg.NewCancelRegistry(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		chatHTTPClient: &http.Client{},
	}
	vectorIndex, err := search.NewVectorIndex(a.embeddingFunc(), cfg.storageRoot)
	if err != nil {
		log.Fatal(err)
	}
	a.vectorIndex = vectorIndex
	go a.runIngestionWorker(context.Background())

	mux := http.NewServeMux()
	a.routes(mux)

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           httpapi.WithCORS(httpapi.WithCorrelation(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("backend listening on %s", cfg.addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (a *app) documentLifecycle() docpkg.Lifecycle {
	return docpkg.Lifecycle{
		Store: a.store,
		Storage: docpkg.LocalStorage{
			Root:     a.config.storageRoot,
			NewToken: randomToken,
		},
		Extractor: docpkg.DocumentExtractor{
			Client:      a.httpClient,
			DocumentURL: a.config.documentURL,
			StorageRoot: a.config.storageRoot,
		},
		ChunkingStrategy: a.chunkingStrategy,
		RetrievalIndex: docpkg.StoreBackedRetrievalIndex{
			Store: a.store,
			Index: a.vectorIndex,
		},
		EmbeddingPurpose: providerPurposeEmbedding,
		RecordMetric:     a.store.RecordMetric,
	}
}

func (a *app) chatClient() *http.Client {
	if a.chatHTTPClient != nil {
		return a.chatHTTPClient
	}
	return a.httpClient
}

func (a *app) chatRequestTimeout() time.Duration {
	if a.config.chatRequestTimeout > 0 {
		return a.config.chatRequestTimeout
	}
	return defaultChatRequestTimeout
}

func loadConfig() config {
	fakeProviders := utils.Env("FAKE_PROVIDERS", "true") == "true"
	return config{
		addr:               utils.Env("BACKEND_ADDR", ":8080"),
		databasePath:       utils.Env("DATABASE_PATH", "/data/office-assistant.db"),
		storageRoot:        utils.Env("STORAGE_ROOT", "/data/files"),
		documentURL:        utils.Env("DOCUMENT_URL", "http://document:8081"),
		ocrURL:             utils.Env("OCR_URL", "http://ocr:8082"),
		fakeProviders:      fakeProviders,
		debugEnvEnabled:    utils.Env("DEBUG_MODE", "false") == "true",
		chatRequestTimeout: durationEnv("CHAT_REQUEST_TIMEOUT", defaultChatRequestTimeout),
		defaultProviders: map[string]domain.ProviderSetting{
			providerPurposeChat: {
				Purpose: providerPurposeChat,
				BaseURL: providerDefaultURL(fakeProviders, "CHAT_PROVIDER_BASE_URL", "http://backend:8080/fake-openai"),
				Model:   utils.Env("CHAT_MODEL", "fake-chat"),
				APIKey:  utils.Env("CHAT_API_KEY", ""),
			},
			providerPurposeEmbedding: {
				Purpose: providerPurposeEmbedding,
				BaseURL: providerDefaultURL(fakeProviders, "EMBEDDING_PROVIDER_BASE_URL", "http://backend:8080/fake-openai"),
				Model:   utils.Env("EMBEDDING_MODEL", "fake-embedding"),
				APIKey:  utils.Env("EMBEDDING_API_KEY", ""),
			},
		},
	}
}

const defaultChatRequestTimeout = 10 * time.Minute

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := utils.Env(key, "")
	if raw == "" {
		return fallback
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid %s=%q, using %s", key, raw, fallback)
		return fallback
	}
	return duration
}

func (a *app) routes(mux *http.ServeMux) {
	httpapi.RegisterRoutes(mux, httpapi.Handlers{
		Health:                  a.health,
		Ready:                   a.ready,
		SetupStatus:             a.setupStatus,
		CreateFirstAdmin:        a.createFirstAdmin,
		Login:                   a.login,
		Logout:                  a.logout,
		Me:                      a.me,
		AdminStatus:             a.adminStatus,
		GetActivity:             a.getActivity,
		GetMetrics:              a.getMetrics,
		GetDebugMode:            a.getDebugMode,
		UpdateDebugMode:         a.updateDebugMode,
		GetProviderSettings:     a.getProviderSettings,
		UpdateProviderSetting:   a.updateProviderSetting,
		ListKnowledgeBases:      a.listKnowledgeBases,
		CreateKnowledgeBase:     a.createKnowledgeBase,
		GetKnowledgeBase:        a.getKnowledgeBase,
		UpdateKnowledgeBase:     a.updateKnowledgeBase,
		DeleteKnowledgeBase:     a.deleteKnowledgeBase,
		ListDocuments:           a.listDocuments,
		SearchDocuments:         a.searchDocuments,
		UploadDocument:          a.uploadDocument,
		DeleteDocument:          a.deleteDocument,
		ReprocessDocument:       a.reprocessDocument,
		CancelDocumentIngestion: a.cancelDocumentIngestion,
		GetExtractedMarkdown:    a.getExtractedMarkdown,
		DownloadDocument:        a.downloadDocument,
		ListChatSessions:        a.listChatSessions,
		GetChatSession:          a.getChatSession,
		DeleteChatSession:       a.deleteChatSession,
		ChatKnowledgeBase:       a.chatKnowledgeBase,
		CancelChatSession:       a.cancelChatSession,
		GetCitationPreview:      a.getCitationPreview,
	})
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Service:   "backend",
		StartedAt: a.startedAt.Format(time.RFC3339),
	})
}

func (a *app) ready(w http.ResponseWriter, _ *http.Request) {
	chat := a.providerDependencyStatus(providerPurposeChat)
	embedding := a.providerDependencyStatus(providerPurposeEmbedding)
	status := "ready"
	if chat.Status != "ready" || embedding.Status != "ready" {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, readinessResponse{
		Status: status,
		Dependencies: map[string]dependencyStatus{
			"document": {
				Status: "configured",
				URL:    a.config.documentURL,
			},
			"ocr": {
				Status: "configured",
				URL:    a.config.ocrURL,
			},
			"chat_model": {
				Status:  chat.Status,
				URL:     chat.URL,
				Mode:    chat.Mode,
				Message: chat.Message,
			},
			"embedding_model": {
				Status:  embedding.Status,
				URL:     embedding.URL,
				Mode:    embedding.Mode,
				Message: embedding.Message,
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	httpapi.WriteJSON(w, status, body)
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	httpapi.WriteError(w, status, code, message, details)
}

func decodeJSON(r *http.Request, target any) error {
	return httpapi.DecodeJSON(r, target)
}

func openStore(path string) (*storepkg.Store, error) {
	return storepkg.Open(path)
}

func providerDefaultURL(fakeProviders bool, key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	if fakeProviders {
		return fallback
	}
	return ""
}
