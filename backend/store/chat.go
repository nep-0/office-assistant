package store

import (
	"context"
	"database/sql"
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

func (s *Store) ListChatSessionsForKnowledgeBase(ctx context.Context, userID, knowledgeBaseID int64) ([]ChatSession, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, knowledge_base_id, title, created_at, updated_at
FROM chat_sessions
WHERE user_id = ?
AND knowledge_base_id = ?
ORDER BY updated_at DESC, created_at DESC
`, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []ChatSession
	for rows.Next() {
		session, err := scanChatSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) DeleteChatSession(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) AppendChatMessages(ctx context.Context, messages []ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, msg := range messages {
		if msg.SessionID != messages[0].SessionID {
			return errors.New("chat transcript batch contains multiple sessions")
		}
		if msg.ToolCallsJSON == "" {
			msg.ToolCallsJSON = "[]"
		}
		if msg.Metadata == "" {
			msg.Metadata = "{}"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO chat_messages (session_id, role, content, tool_calls_json, tool_call_id, metadata_json)
VALUES (?, ?, ?, ?, ?, ?)
`, msg.SessionID, msg.Role, msg.Content, msg.ToolCallsJSON, msg.ToolCallID, msg.Metadata); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
UPDATE chat_sessions
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, messages[0].SessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListChatMessages(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, tool_calls_json, tool_call_id, metadata_json, created_at
FROM (
	SELECT id, session_id, role, content, tool_calls_json, tool_call_id, metadata_json, created_at
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

func (s *Store) ListAllChatMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, tool_calls_json, tool_call_id, metadata_json, created_at
FROM chat_messages
WHERE session_id = ?
ORDER BY id ASC
`, sessionID)
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
