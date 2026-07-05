package store

import (
	"context"
	"errors"
)

func (s *Store) CreateChatSession(ctx context.Context, session ChatSession) (ChatSession, error) {
	if session.ID == "" {
		return ChatSession{}, errors.New("chat session id is required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_sessions (id, user_id, knowledge_base_id, title)
VALUES (?, ?, ?, ?)
`, session.ID, session.UserID, session.KnowledgeBaseID, session.Title)
	if err != nil {
		return ChatSession{}, err
	}
	return s.FindChatSession(ctx, session.ID)
}

func (s *Store) FindChatSession(ctx context.Context, id string) (ChatSession, error) {
	return scanChatSession(s.db.QueryRowContext(ctx, `
SELECT id, user_id, knowledge_base_id, title, created_at, updated_at
FROM chat_sessions
WHERE id = ?
`, id))
}

func (s *Store) AppendChatMessage(ctx context.Context, msg ChatMessage) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_messages (session_id, role, content, metadata_json)
VALUES (?, ?, ?, ?)
`, msg.SessionID, msg.Role, msg.Content, msg.Metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE chat_sessions
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, msg.SessionID)
	return err
}

func (s *Store) ListChatMessages(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, metadata_json, created_at
FROM (
	SELECT id, session_id, role, content, metadata_json, created_at
	FROM chat_messages
	WHERE session_id = ?
	ORDER BY id DESC
	LIMIT ?
)
ORDER BY id ASC
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ChatMessage
	for rows.Next() {
		msg, err := scanChatMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}
