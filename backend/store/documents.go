package store

import "context"

func (s *Store) CreateDocument(ctx context.Context, doc DocumentRecord) (DocumentRecord, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO documents (
	knowledge_base_id,
	owner_user_id,
	original_filename,
	display_name,
	content_type,
	size_bytes,
	sha256,
	storage_key,
	status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, doc.KnowledgeBaseID, doc.OwnerID, doc.OriginalFilename, doc.DisplayName, doc.ContentType, doc.SizeBytes, doc.SHA256, doc.StorageKey, doc.Status)
	if err != nil {
		return DocumentRecord{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return DocumentRecord{}, err
	}
	return s.FindDocumentByID(ctx, id)
}

func (s *Store) ListDocumentsForKnowledgeBase(ctx context.Context, knowledgeBaseID int64) ([]DocumentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE knowledge_base_id = ? AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
`, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []DocumentRecord
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *Store) FindDocumentDuplicateInKnowledgeBase(ctx context.Context, knowledgeBaseID int64, hash string) (DocumentRecord, error) {
	return scanDocument(s.db.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE knowledge_base_id = ? AND sha256 = ? AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1
`, knowledgeBaseID, hash))
}

func (s *Store) FindDocumentByID(ctx context.Context, id int64) (DocumentRecord, error) {
	return scanDocument(s.db.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE id = ? AND deleted_at IS NULL
`, id))
}

type DocumentSearchFilter struct {
	Query       string
	Status      string
	ContentType string
	DateFrom    string
	DateTo      string
}

func (s *Store) SearchDocuments(ctx context.Context, knowledgeBaseID int64, filter DocumentSearchFilter) ([]DocumentRecord, error) {
	query := `
SELECT documents.id, documents.knowledge_base_id, documents.owner_user_id, documents.original_filename, documents.display_name, documents.content_type, documents.size_bytes, documents.sha256, documents.storage_key, documents.status, documents.error_code, documents.error_message, documents.created_at, documents.updated_at
FROM documents
`
	args := []any{knowledgeBaseID}
	if filter.Query != "" {
		query += `JOIN document_search_fts ON document_search_fts.rowid = documents.id
`
		args = append(args, filter.Query)
	}
	query += `WHERE documents.knowledge_base_id = ? AND documents.deleted_at IS NULL
`
	if filter.Query != "" {
		query += `AND document_search_fts MATCH ?
`
	}
	if filter.Status != "" {
		query += `AND documents.status = ?
`
		args = append(args, filter.Status)
	}
	if filter.ContentType != "" {
		query += `AND documents.content_type = ?
`
		args = append(args, filter.ContentType)
	}
	if filter.DateFrom != "" {
		query += `AND documents.created_at >= ?
`
		args = append(args, filter.DateFrom)
	}
	if filter.DateTo != "" {
		query += `AND documents.created_at <= ?
`
		args = append(args, filter.DateTo)
	}
	query += `ORDER BY documents.updated_at DESC, documents.id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []DocumentRecord
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *Store) ListIndexedChunks(ctx context.Context) ([]DocumentChunk, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT document_chunks.id, document_chunks.document_id, document_chunks.document_version_id, document_chunks.chunk_no, document_chunks.content, document_chunks.heading_path, document_chunks.source_anchor_json, document_chunks.token_count, document_chunks.embedding_model, document_chunks.indexing_status, document_chunks.created_at
FROM document_chunks
JOIN documents ON documents.id = document_chunks.document_id
JOIN knowledge_bases ON knowledge_bases.id = documents.knowledge_base_id
JOIN document_versions ON document_versions.id = document_chunks.document_version_id
WHERE document_chunks.indexing_status = 'indexed'
	AND documents.deleted_at IS NULL
	AND knowledge_bases.deleted_at IS NULL
	AND document_versions.id = (
		SELECT MAX(id)
		FROM document_versions latest
		WHERE latest.document_id = document_chunks.document_id
	)
ORDER BY document_chunks.id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []DocumentChunk
	for rows.Next() {
		chunk, err := scanDocumentChunk(rows)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *Store) FindRetrievalChunk(ctx context.Context, chunkID, knowledgeBaseID int64) (RetrievalChunk, error) {
	var chunk RetrievalChunk
	err := s.db.QueryRowContext(ctx, `
SELECT document_chunks.id, documents.id, documents.display_name, document_chunks.content, document_chunks.heading_path, document_chunks.source_anchor_json
FROM document_chunks
JOIN documents ON documents.id = document_chunks.document_id
JOIN knowledge_bases ON knowledge_bases.id = documents.knowledge_base_id
JOIN document_versions ON document_versions.id = document_chunks.document_version_id
WHERE document_chunks.id = ?
	AND documents.knowledge_base_id = ?
	AND document_chunks.indexing_status = 'indexed'
	AND documents.deleted_at IS NULL
	AND knowledge_bases.deleted_at IS NULL
	AND document_versions.id = (
		SELECT MAX(id)
		FROM document_versions latest
		WHERE latest.document_id = document_chunks.document_id
	)
`, chunkID, knowledgeBaseID).Scan(&chunk.ChunkID, &chunk.DocumentID, &chunk.DocumentName, &chunk.Content, &chunk.HeadingPath, &chunk.SourceAnchorJSON)
	return chunk, err
}
