package knowledge

import (
	"time"

	"office-assistant/backend/domain"
)

type Response struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	OwnerID    int64  `json:"owner_id"`
	OwnerName  string `json:"owner_name"`
	CanWrite   bool   `json:"can_write"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func ToResponse(kb domain.KnowledgeBase, current domain.User) Response {
	return Response{
		ID:         kb.ID,
		Name:       kb.Name,
		Visibility: kb.Visibility,
		OwnerID:    kb.OwnerID,
		OwnerName:  kb.OwnerName,
		CanWrite:   CanModify(current, kb),
		CreatedAt:  kb.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  kb.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
