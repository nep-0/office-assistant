package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type app struct {
	startedAt        time.Time
	config           config
	store            *store
	httpClient       *http.Client
	chunkingStrategy ChunkingStrategy
	vectorIndex      *vectorIndex
	activeChats      map[string]context.CancelFunc
	activeChatsMu    sync.Mutex
}

type config struct {
	addr             string
	databasePath     string
	storageRoot      string
	documentURL      string
	ocrURL           string
	fakeProviders    bool
	defaultProviders map[string]providerSetting
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
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

func main() {
	cfg := loadConfig()
	store, err := openStore(cfg.databasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err := store.ensureProviderDefaults(context.Background(), cfg.defaultProviders); err != nil {
		log.Fatal(err)
	}
	a := &app{
		startedAt:        time.Now().UTC(),
		config:           cfg,
		store:            store,
		chunkingStrategy: markdownChunkingStrategy{},
		activeChats:      make(map[string]context.CancelFunc),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	vectorIndex, err := newVectorIndex(a.embeddingFunc())
	if err != nil {
		log.Fatal(err)
	}
	a.vectorIndex = vectorIndex
	if err := a.rebuildVectorIndex(context.Background()); err != nil {
		log.Fatal(err)
	}
	go a.runIngestionWorker(context.Background())

	mux := http.NewServeMux()
	a.routes(mux)

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("backend listening on %s", cfg.addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func loadConfig() config {
	fakeProviders := env("FAKE_PROVIDERS", "true") == "true"
	return config{
		addr:          env("BACKEND_ADDR", ":8080"),
		databasePath:  env("DATABASE_PATH", "/data/office-assistant.db"),
		storageRoot:   env("STORAGE_ROOT", "/data/files"),
		documentURL:   env("DOCUMENT_URL", "http://document:8081"),
		ocrURL:        env("OCR_URL", "http://ocr:8082"),
		fakeProviders: fakeProviders,
		defaultProviders: map[string]providerSetting{
			providerPurposeChat: {
				Purpose: providerPurposeChat,
				BaseURL: providerDefaultURL(fakeProviders, "CHAT_PROVIDER_BASE_URL", "http://backend:8080/fake-openai"),
				Model:   env("CHAT_MODEL", "fake-chat"),
				APIKey:  env("CHAT_API_KEY", ""),
			},
			providerPurposeEmbedding: {
				Purpose: providerPurposeEmbedding,
				BaseURL: providerDefaultURL(fakeProviders, "EMBEDDING_PROVIDER_BASE_URL", "http://backend:8080/fake-openai"),
				Model:   env("EMBEDDING_MODEL", "fake-embedding"),
				APIKey:  env("EMBEDDING_API_KEY", ""),
			},
		},
	}
}

func (a *app) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/ready", a.ready)
	mux.HandleFunc("GET /api/setup/status", a.setupStatus)
	mux.HandleFunc("POST /api/setup", a.createFirstAdmin)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", a.me)
	mux.HandleFunc("GET /api/admin/status", a.adminStatus)
	mux.HandleFunc("GET /api/admin/provider-settings", a.getProviderSettings)
	mux.HandleFunc("PUT /api/admin/provider-settings/{purpose}", a.updateProviderSetting)
	mux.HandleFunc("GET /api/knowledge-bases", a.listKnowledgeBases)
	mux.HandleFunc("POST /api/knowledge-bases", a.createKnowledgeBase)
	mux.HandleFunc("GET /api/knowledge-bases/{id}", a.getKnowledgeBase)
	mux.HandleFunc("PUT /api/knowledge-bases/{id}", a.updateKnowledgeBase)
	mux.HandleFunc("DELETE /api/knowledge-bases/{id}", a.deleteKnowledgeBase)
	mux.HandleFunc("GET /api/knowledge-bases/{id}/documents", a.listDocuments)
	mux.HandleFunc("GET /api/knowledge-bases/{id}/documents/search", a.searchDocuments)
	mux.HandleFunc("POST /api/knowledge-bases/{id}/documents/upload", a.uploadDocument)
	mux.HandleFunc("DELETE /api/documents/{id}", a.deleteDocument)
	mux.HandleFunc("POST /api/documents/{id}/reprocess", a.reprocessDocument)
	mux.HandleFunc("POST /api/documents/{id}/ingestion/cancel", a.cancelDocumentIngestion)
	mux.HandleFunc("GET /api/documents/{id}/extracted-markdown", a.getExtractedMarkdown)
	mux.HandleFunc("POST /api/knowledge-bases/{id}/chat", a.chatKnowledgeBase)
	mux.HandleFunc("POST /api/chat-sessions/{id}/cancel", a.cancelChatSession)
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /ready", a.ready)
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

	writeJSON(w, http.StatusOK, readinessResponse{
		Status: "ready",
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
				Status: chat.Status,
				URL:    chat.URL,
				Mode:   chat.Mode,
			},
			"embedding_model": {
				Status: embedding.Status,
				URL:    embedding.URL,
				Mode:   embedding.Mode,
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, apiError{
		Code:    code,
		Message: message,
		Details: details,
	})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func openStore(path string) (*store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func notFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
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
