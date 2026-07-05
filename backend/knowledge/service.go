package knowledge

import "office-assistant/backend/domain"

const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"
)

func CanRead(current domain.User, kb domain.KnowledgeBase) bool {
	return current.Role == "admin" || kb.Visibility == VisibilityPublic || kb.OwnerID == current.ID
}

func CanModify(current domain.User, kb domain.KnowledgeBase) bool {
	return current.Role == "admin" || kb.OwnerID == current.ID
}

func ValidVisibility(visibility string) bool {
	return visibility == VisibilityPrivate || visibility == VisibilityPublic
}
