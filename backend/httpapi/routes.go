package httpapi

import "net/http"

type Handlers struct {
	Health                  http.HandlerFunc
	Ready                   http.HandlerFunc
	SetupStatus             http.HandlerFunc
	CreateFirstAdmin        http.HandlerFunc
	Login                   http.HandlerFunc
	Logout                  http.HandlerFunc
	Me                      http.HandlerFunc
	AdminStatus             http.HandlerFunc
	GetActivity             http.HandlerFunc
	GetMetrics              http.HandlerFunc
	GetDebugMode            http.HandlerFunc
	UpdateDebugMode         http.HandlerFunc
	ListUsers               http.HandlerFunc
	CreateUser              http.HandlerFunc
	UpdateUser              http.HandlerFunc
	DeleteUser              http.HandlerFunc
	GetProviderSettings     http.HandlerFunc
	UpdateProviderSetting   http.HandlerFunc
	ListKnowledgeBases      http.HandlerFunc
	CreateKnowledgeBase     http.HandlerFunc
	GetKnowledgeBase        http.HandlerFunc
	UpdateKnowledgeBase     http.HandlerFunc
	DeleteKnowledgeBase     http.HandlerFunc
	ListDocuments           http.HandlerFunc
	SearchDocuments         http.HandlerFunc
	UploadDocument          http.HandlerFunc
	DeleteDocument          http.HandlerFunc
	ReprocessDocument       http.HandlerFunc
	CancelDocumentIngestion http.HandlerFunc
	GetExtractedMarkdown    http.HandlerFunc
	DownloadDocument        http.HandlerFunc
	ListChatSessions        http.HandlerFunc
	GetChatSession          http.HandlerFunc
	DeleteChatSession       http.HandlerFunc
	ChatKnowledgeBase       http.HandlerFunc
	CancelChatSession       http.HandlerFunc
	GetCitationPreview      http.HandlerFunc
}

func RegisterRoutes(mux *http.ServeMux, h Handlers) {
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/ready", h.Ready)
	mux.HandleFunc("GET /api/setup/status", h.SetupStatus)
	mux.HandleFunc("POST /api/setup", h.CreateFirstAdmin)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Logout)
	mux.HandleFunc("GET /api/auth/me", h.Me)
	mux.HandleFunc("GET /api/admin/status", h.AdminStatus)
	mux.HandleFunc("GET /api/admin/activity", h.GetActivity)
	mux.HandleFunc("GET /api/admin/metrics", h.GetMetrics)
	mux.HandleFunc("GET /api/admin/debug", h.GetDebugMode)
	mux.HandleFunc("PUT /api/admin/debug", h.UpdateDebugMode)
	mux.HandleFunc("GET /api/admin/users", h.ListUsers)
	mux.HandleFunc("POST /api/admin/users", h.CreateUser)
	mux.HandleFunc("PUT /api/admin/users/{id}", h.UpdateUser)
	mux.HandleFunc("DELETE /api/admin/users/{id}", h.DeleteUser)
	mux.HandleFunc("GET /api/admin/provider-settings", h.GetProviderSettings)
	mux.HandleFunc("PUT /api/admin/provider-settings/{purpose}", h.UpdateProviderSetting)
	mux.HandleFunc("GET /api/knowledge-bases", h.ListKnowledgeBases)
	mux.HandleFunc("POST /api/knowledge-bases", h.CreateKnowledgeBase)
	mux.HandleFunc("GET /api/knowledge-bases/{id}", h.GetKnowledgeBase)
	mux.HandleFunc("PUT /api/knowledge-bases/{id}", h.UpdateKnowledgeBase)
	mux.HandleFunc("DELETE /api/knowledge-bases/{id}", h.DeleteKnowledgeBase)
	mux.HandleFunc("GET /api/knowledge-bases/{id}/documents", h.ListDocuments)
	mux.HandleFunc("GET /api/knowledge-bases/{id}/documents/search", h.SearchDocuments)
	mux.HandleFunc("POST /api/knowledge-bases/{id}/documents/upload", h.UploadDocument)
	mux.HandleFunc("DELETE /api/documents/{id}", h.DeleteDocument)
	mux.HandleFunc("POST /api/documents/{id}/reprocess", h.ReprocessDocument)
	mux.HandleFunc("POST /api/documents/{id}/ingestion/cancel", h.CancelDocumentIngestion)
	mux.HandleFunc("GET /api/documents/{id}/extracted-markdown", h.GetExtractedMarkdown)
	mux.HandleFunc("GET /api/documents/{id}/download", h.DownloadDocument)
	mux.HandleFunc("GET /api/knowledge-bases/{id}/chat-sessions", h.ListChatSessions)
	mux.HandleFunc("GET /api/chat-sessions/{id}", h.GetChatSession)
	mux.HandleFunc("DELETE /api/chat-sessions/{id}", h.DeleteChatSession)
	mux.HandleFunc("POST /api/knowledge-bases/{id}/chat", h.ChatKnowledgeBase)
	mux.HandleFunc("POST /api/chat-sessions/{id}/cancel", h.CancelChatSession)
	mux.HandleFunc("GET /api/chat-sessions/{id}/citations/{citation}/preview", h.GetCitationPreview)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)
}
