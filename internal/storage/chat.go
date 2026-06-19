package storage

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) SaveChatMessage(ctx context.Context, sessionID, messageID, role, content string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `insert or ignore into chat_sessions (id, knowledge_base_id, created_at) values (?, ?, ?)`, sessionID, "demo", now)
	if err != nil {
		return fmt.Errorf("save chat session: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `insert into chat_messages (id, session_id, role, content, created_at) values (?, ?, ?, ?, ?)`, messageID, sessionID, role, content, now)
	if err != nil {
		return fmt.Errorf("save chat message: %w", err)
	}
	return nil
}

func (s *Store) SaveCitation(ctx context.Context, citation CitationRecord) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `insert into citations (
		id, message_id, document_id, chunk_id, source_file_name, page_number, preview, score, created_at
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		citation.ID,
		citation.MessageID,
		citation.DocumentID,
		citation.ChunkID,
		citation.SourceFileName,
		citation.PageNumber,
		citation.Preview,
		citation.Score,
		now,
	)
	if err != nil {
		return fmt.Errorf("save citation: %w", err)
	}
	return nil
}
