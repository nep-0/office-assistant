package chat

import (
	"context"
	"database/sql"

	"office-assistant/backend/domain"
	"office-assistant/backend/store"
)

type TokenFunc func() (string, error)

func ResolveSession(ctx context.Context, st *store.Store, current domain.User, kb domain.KnowledgeBase, sessionID, message string, newToken TokenFunc) (domain.ChatSession, error) {
	if sessionID != "" {
		sessionRecord, err := st.FindChatSession(ctx, sessionID)
		if err != nil {
			return domain.ChatSession{}, err
		}
		if sessionRecord.UserID != current.ID || sessionRecord.KnowledgeBaseID != kb.ID {
			return domain.ChatSession{}, sql.ErrNoRows
		}
		return sessionRecord, nil
	}

	id, err := newToken()
	if err != nil {
		return domain.ChatSession{}, err
	}
	title := message
	if len(title) > 80 {
		title = title[:80]
	}
	return st.CreateChatSession(ctx, domain.ChatSession{
		ID:              id,
		UserID:          current.ID,
		KnowledgeBaseID: kb.ID,
		Title:           title,
	})
}

func FindOwnedSession(ctx context.Context, st *store.Store, current domain.User, sessionID string) (domain.ChatSession, error) {
	sessionRecord, err := st.FindChatSession(ctx, sessionID)
	if err != nil {
		return domain.ChatSession{}, err
	}
	if sessionRecord.UserID != current.ID {
		return domain.ChatSession{}, sql.ErrNoRows
	}
	return sessionRecord, nil
}
