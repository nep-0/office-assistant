package store

import (
	"context"
	"database/sql"
	"time"
)

func scanUser(row rowScanner) (User, error) {
	var u User
	var createdAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt); err != nil {
		return User{}, err
	}
	parsed, err := parseSQLiteTime(createdAt)
	if err != nil {
		return User{}, err
	}
	u.CreatedAt = parsed
	return u, nil
}

func scanProviderSetting(row rowScanner) (ProviderSetting, error) {
	var setting ProviderSetting
	var updatedAt string
	if err := row.Scan(&setting.Purpose, &setting.BaseURL, &setting.Model, &setting.APIKey, &updatedAt); err != nil {
		return ProviderSetting{}, err
	}
	parsed, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return ProviderSetting{}, err
	}
	setting.UpdatedAt = parsed
	return setting, nil
}

func scanKnowledgeBase(row rowScanner) (KnowledgeBase, error) {
	var kb KnowledgeBase
	var createdAt string
	var updatedAt string
	if err := row.Scan(&kb.ID, &kb.OwnerID, &kb.OwnerName, &kb.Name, &kb.Visibility, &createdAt, &updatedAt); err != nil {
		return KnowledgeBase{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return KnowledgeBase{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return KnowledgeBase{}, err
	}
	kb.CreatedAt = created
	kb.UpdatedAt = updated
	return kb, nil
}

func scanDocument(row rowScanner) (DocumentRecord, error) {
	var doc DocumentRecord
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&doc.ID,
		&doc.KnowledgeBaseID,
		&doc.OwnerID,
		&doc.OriginalFilename,
		&doc.DisplayName,
		&doc.ContentType,
		&doc.SizeBytes,
		&doc.SHA256,
		&doc.StorageKey,
		&doc.Status,
		&doc.ErrorCode,
		&doc.ErrorMessage,
		&createdAt,
		&updatedAt,
	); err != nil {
		return DocumentRecord{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return DocumentRecord{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return DocumentRecord{}, err
	}
	doc.CreatedAt = created
	doc.UpdatedAt = updated
	return doc, nil
}

func scanIngestionJob(row rowScanner) (IngestionJob, error) {
	var job IngestionJob
	var createdAt string
	var updatedAt string
	if err := row.Scan(&job.ID, &job.DocumentID, &job.Status, &job.Attempts, &job.MaxAttempts, &job.ErrorCode, &job.ErrorMessage, &createdAt, &updatedAt); err != nil {
		return IngestionJob{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return IngestionJob{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return IngestionJob{}, err
	}
	job.CreatedAt = created
	job.UpdatedAt = updated
	return job, nil
}

func scanDocumentVersion(row rowScanner) (DocumentVersion, error) {
	var version DocumentVersion
	var createdAt string
	if err := row.Scan(&version.ID, &version.DocumentID, &version.VersionNo, &version.MarkdownStorageKey, &version.SchemaVersion, &version.MetadataJSON, &version.IndexingStatus, &version.EmbeddingModel, &createdAt); err != nil {
		return DocumentVersion{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return DocumentVersion{}, err
	}
	version.CreatedAt = created
	return version, nil
}

func scanDocumentChunk(row rowScanner) (DocumentChunk, error) {
	var chunk DocumentChunk
	var createdAt string
	if err := row.Scan(&chunk.ID, &chunk.DocumentID, &chunk.DocumentVersionID, &chunk.ChunkNo, &chunk.Content, &chunk.HeadingPath, &chunk.SourceAnchorJSON, &chunk.TokenCount, &chunk.EmbeddingModel, &chunk.IndexingStatus, &createdAt); err != nil {
		return DocumentChunk{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return DocumentChunk{}, err
	}
	chunk.CreatedAt = created
	return chunk, nil
}

func scanChatSession(row rowScanner) (ChatSession, error) {
	var session ChatSession
	var createdAt string
	var updatedAt string
	if err := row.Scan(&session.ID, &session.UserID, &session.KnowledgeBaseID, &session.Title, &createdAt, &updatedAt); err != nil {
		return ChatSession{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return ChatSession{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return ChatSession{}, err
	}
	session.CreatedAt = created
	session.UpdatedAt = updated
	return session, nil
}

func scanChatMessage(row rowScanner) (ChatMessage, error) {
	var msg ChatMessage
	var createdAt string
	if err := row.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.ToolCallsJSON, &msg.ToolCallID, &msg.Metadata, &createdAt); err != nil {
		return ChatMessage{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return ChatMessage{}, err
	}
	msg.CreatedAt = created
	return msg, nil
}

func scanActivityEvent(row rowScanner) (ActivityEvent, error) {
	var event ActivityEvent
	var createdAt string
	if err := row.Scan(&event.ID, &event.UserID, &event.EventType, &event.EntityType, &event.EntityID, &event.Details, &createdAt); err != nil {
		return ActivityEvent{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return ActivityEvent{}, err
	}
	event.CreatedAt = created
	return event, nil
}

func scanWorkflowMetric(row rowScanner) (WorkflowMetric, error) {
	var metric WorkflowMetric
	var createdAt string
	if err := row.Scan(&metric.ID, &metric.Name, &metric.ValueMS, &metric.Count, &metric.Details, &createdAt); err != nil {
		return WorkflowMetric{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return WorkflowMetric{}, err
	}
	metric.CreatedAt = created
	return metric, nil
}

func upsertDocumentSearchTx(ctx context.Context, tx *sql.Tx, doc DocumentRecord, markdownStorageKey string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_search_fts WHERE rowid = ?`, doc.ID); err != nil {
		return err
	}
	var extractedText string
	rows, err := tx.QueryContext(ctx, `
SELECT content
FROM document_chunks
WHERE document_id = ? AND indexing_status = 'indexed'
ORDER BY chunk_no ASC
`, doc.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return err
		}
		if extractedText != "" {
			extractedText += "\n\n"
		}
		extractedText += content
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = markdownStorageKey
	_, err = tx.ExecContext(ctx, `
INSERT INTO document_search_fts (rowid, original_filename, display_name, content_type, status, created_at, extracted_text)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, doc.ID, doc.OriginalFilename, doc.DisplayName, doc.ContentType, documentStatusReady, doc.CreatedAt.UTC().Format(time.RFC3339), extractedText)
	return err
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
