package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateDocument(ctx context.Context, doc Document) (Document, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if doc.CreatedAt == "" {
		doc.CreatedAt = now
	}
	if doc.UpdatedAt == "" {
		doc.UpdatedAt = now
	}
	if doc.Status == "" {
		doc.Status = "pending"
	}
	_, err := s.db.ExecContext(ctx, `insert into documents (
		id, knowledge_base_id, uploaded_by, original_name, storage_path, content_type, size_bytes, sha256, status, status_reason, created_at, updated_at
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID,
		doc.KnowledgeBaseID,
		doc.UploadedBy,
		doc.OriginalName,
		doc.StoragePath,
		doc.ContentType,
		doc.SizeBytes,
		doc.SHA256,
		doc.Status,
		doc.StatusReason,
		doc.CreatedAt,
		doc.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Document{}, fmt.Errorf("duplicate document upload: %w", err)
		}
		return Document{}, fmt.Errorf("create document: %w", err)
	}
	return doc, nil
}

func (s *Store) ListDocuments(ctx context.Context, knowledgeBaseID string) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx, `select
		id, knowledge_base_id, uploaded_by, original_name, storage_path, content_type, size_bytes, sha256, status, status_reason, created_at, updated_at
		from documents where knowledge_base_id = ? order by created_at desc`, knowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	docs := []Document{}
	for rows.Next() {
		var doc Document
		if err := rows.Scan(
			&doc.ID,
			&doc.KnowledgeBaseID,
			&doc.UploadedBy,
			&doc.OriginalName,
			&doc.StoragePath,
			&doc.ContentType,
			&doc.SizeBytes,
			&doc.SHA256,
			&doc.Status,
			&doc.StatusReason,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return docs, nil
}

func (s *Store) UpdateDocumentStatus(ctx context.Context, documentID, status, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `update documents set status = ?, status_reason = ?, updated_at = ? where id = ?`,
		status, reason, now, documentID)
	if err != nil {
		return fmt.Errorf("update document status: %w", err)
	}
	return nil
}

func (s *Store) ReplaceDocumentChunks(ctx context.Context, documentID string, chunks []Chunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace chunks transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `delete from chunks where document_id = ?`, documentID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, chunk := range chunks {
		if chunk.CreatedAt == "" {
			chunk.CreatedAt = now
		}
		_, err := tx.ExecContext(ctx, `insert into chunks (
			id, document_id, knowledge_base_id, content, source_file_name, page_number, chunk_index, content_type, token_or_char_count, metadata_json, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			chunk.ID,
			chunk.DocumentID,
			chunk.KnowledgeBaseID,
			chunk.Content,
			chunk.SourceFileName,
			chunk.PageNumber,
			chunk.ChunkIndex,
			chunk.ContentType,
			chunk.TokenOrCharCount,
			chunk.MetadataJSON,
			chunk.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace chunks transaction: %w", err)
	}
	return nil
}
