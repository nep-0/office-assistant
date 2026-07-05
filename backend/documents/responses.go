package documents

import (
	"time"

	"office-assistant/backend/domain"
)

type Response struct {
	ID               int64  `json:"id"`
	KnowledgeBaseID  int64  `json:"knowledge_base_id"`
	OriginalFilename string `json:"original_filename"`
	DisplayName      string `json:"display_name"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	SHA256           string `json:"sha256"`
	Status           string `json:"status"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func ToResponse(doc domain.DocumentRecord) Response {
	return Response{
		ID:               doc.ID,
		KnowledgeBaseID:  doc.KnowledgeBaseID,
		OriginalFilename: doc.OriginalFilename,
		DisplayName:      doc.DisplayName,
		ContentType:      doc.ContentType,
		SizeBytes:        doc.SizeBytes,
		SHA256:           doc.SHA256,
		Status:           doc.Status,
		ErrorCode:        doc.ErrorCode,
		ErrorMessage:     doc.ErrorMessage,
		CreatedAt:        doc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        doc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
