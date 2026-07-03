package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type app struct {
	startedAt time.Time
	config    config
}

type config struct {
	addr          string
	documentURL   string
	ocrURL        string
	fakeProviders bool
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
	a := &app{
		startedAt: time.Now().UTC(),
		config:    cfg,
	}

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
	return config{
		addr:          env("BACKEND_ADDR", ":8080"),
		documentURL:   env("DOCUMENT_URL", "http://document:8081"),
		ocrURL:        env("OCR_URL", "http://ocr:8082"),
		fakeProviders: env("FAKE_PROVIDERS", "true") == "true",
	}
}

func (a *app) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/ready", a.ready)
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
	providerMode := "external"
	if a.config.fakeProviders {
		providerMode = "fake"
	}

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
				Status: "ready",
				Mode:   providerMode,
			},
			"embedding_model": {
				Status: "ready",
				Mode:   providerMode,
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
