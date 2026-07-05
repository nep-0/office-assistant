package store

import (
	"context"
	"database/sql"
)

func (s *Store) CreateIngestionJob(ctx context.Context, documentID int64) (IngestionJob, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO ingestion_jobs (document_id, status)
VALUES (?, 'pending')
`, documentID)
	if err != nil {
		return IngestionJob{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return IngestionJob{}, err
	}
	return s.FindIngestionJobByID(ctx, id)
}

func (s *Store) FindLatestIngestionJobForDocument(ctx context.Context, documentID int64) (IngestionJob, error) {
	return scanIngestionJob(s.db.QueryRowContext(ctx, `
SELECT id, document_id, status, attempts, max_attempts, error_code, error_message, created_at, updated_at
FROM ingestion_jobs
WHERE document_id = ?
ORDER BY id DESC
LIMIT 1
`, documentID))
}

func (s *Store) FindIngestionJobByID(ctx context.Context, id int64) (IngestionJob, error) {
	return scanIngestionJob(s.db.QueryRowContext(ctx, `
SELECT id, document_id, status, attempts, max_attempts, error_code, error_message, created_at, updated_at
FROM ingestion_jobs
WHERE id = ?
`, id))
}

func (s *Store) ClaimNextIngestionJob(ctx context.Context) (IngestionJob, DocumentRecord, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestionJob{}, DocumentRecord{}, false, err
	}
	defer tx.Rollback()

	job, err := scanIngestionJob(tx.QueryRowContext(ctx, `
SELECT id, document_id, status, attempts, max_attempts, error_code, error_message, created_at, updated_at
FROM ingestion_jobs
WHERE status IN ('pending', 'cancel_requested')
ORDER BY id ASC
LIMIT 1
`))
	if err != nil {
		if notFound(err) {
			return IngestionJob{}, DocumentRecord{}, false, nil
		}
		return IngestionJob{}, DocumentRecord{}, false, err
	}

	if job.Status == ingestionJobCancelRequested {
		if err := updateIngestionJobTx(ctx, tx, job.ID, ingestionJobCancelled, job.Attempts, "cancelled", "ingestion cancelled"); err != nil {
			return IngestionJob{}, DocumentRecord{}, false, err
		}
		if err := updateDocumentStatusTx(ctx, tx, job.DocumentID, documentStatusCancelled, "cancelled", "ingestion cancelled"); err != nil {
			return IngestionJob{}, DocumentRecord{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return IngestionJob{}, DocumentRecord{}, false, err
		}
		return IngestionJob{}, DocumentRecord{}, false, nil
	}

	attempts := job.Attempts + 1
	if _, err := tx.ExecContext(ctx, `
UPDATE ingestion_jobs
SET status = 'processing', attempts = ?, error_code = '', error_message = '', updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, attempts, job.ID); err != nil {
		return IngestionJob{}, DocumentRecord{}, false, err
	}
	if err := updateDocumentStatusTx(ctx, tx, job.DocumentID, documentStatusProcessing, "", ""); err != nil {
		return IngestionJob{}, DocumentRecord{}, false, err
	}
	doc, err := scanDocument(tx.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE id = ? AND deleted_at IS NULL
`, job.DocumentID))
	if err != nil {
		return IngestionJob{}, DocumentRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return IngestionJob{}, DocumentRecord{}, false, err
	}
	job.Status = ingestionJobProcessing
	job.Attempts = attempts
	return job, doc, true, nil
}

func (s *Store) CompleteIngestionJob(ctx context.Context, job IngestionJob, doc DocumentRecord, version DocumentVersion, chunks []DocumentChunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	versionNo, err := nextDocumentVersionNo(ctx, tx, doc.ID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE document_chunks
SET indexing_status = 'superseded'
WHERE document_id = ? AND indexing_status = 'indexed'
`, doc.ID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO document_versions (document_id, version_no, markdown_storage_key, schema_version, metadata_json, indexing_status, embedding_model)
VALUES (?, ?, ?, ?, ?, 'indexed', ?)
`, doc.ID, versionNo, version.MarkdownStorageKey, version.SchemaVersion, version.MetadataJSON, version.EmbeddingModel)
	if err != nil {
		return err
	}
	versionID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for i, chunk := range chunks {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO document_chunks (
	document_id,
	document_version_id,
	chunk_no,
	content,
	heading_path,
	source_anchor_json,
	token_count,
	embedding_model,
	indexing_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'indexed')
`, doc.ID, versionID, i+1, chunk.Content, chunk.HeadingPath, chunk.SourceAnchorJSON, chunk.TokenCount, version.EmbeddingModel); err != nil {
			return err
		}
	}
	if err := upsertDocumentSearchTx(ctx, tx, doc, version.MarkdownStorageKey); err != nil {
		return err
	}
	if err := updateIngestionJobTx(ctx, tx, job.ID, ingestionJobSucceeded, job.Attempts, "", ""); err != nil {
		return err
	}
	if err := updateDocumentStatusTx(ctx, tx, doc.ID, documentStatusReady, "", ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReprocessDocument(ctx context.Context, documentID int64) (IngestionJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestionJob{}, err
	}
	defer tx.Rollback()
	if err := updateDocumentStatusTx(ctx, tx, documentID, documentStatusPending, "", ""); err != nil {
		return IngestionJob{}, err
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO ingestion_jobs (document_id, status)
VALUES (?, 'pending')
`, documentID)
	if err != nil {
		return IngestionJob{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return IngestionJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return IngestionJob{}, err
	}
	return s.FindIngestionJobByID(ctx, id)
}

func (s *Store) DeleteDocument(ctx context.Context, documentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE documents
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
`, documentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE document_chunks
SET indexing_status = 'deleted'
WHERE document_id = ? AND indexing_status = 'indexed'
`, documentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_search_fts WHERE rowid = ?`, documentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailIngestionJob(ctx context.Context, job IngestionJob, code, message string) error {
	status := ingestionJobFailed
	docStatus := documentStatusFailed
	if job.Attempts < job.MaxAttempts {
		status = ingestionJobPending
		docStatus = documentStatusPending
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateIngestionJobTx(ctx, tx, job.ID, status, job.Attempts, code, message); err != nil {
		return err
	}
	if err := updateDocumentStatusTx(ctx, tx, job.DocumentID, docStatus, code, message); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CancelIngestionForDocument(ctx context.Context, documentID int64) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ingestion_jobs
SET status = CASE WHEN status = 'processing' THEN 'cancel_requested' ELSE 'cancel_requested' END,
	error_code = 'cancel_requested',
	error_message = 'ingestion cancellation requested',
	updated_at = CURRENT_TIMESTAMP
WHERE document_id = ? AND status IN ('pending', 'processing')
`, documentID)
	return err
}

func (s *Store) FindLatestDocumentVersion(ctx context.Context, documentID int64) (DocumentVersion, error) {
	return scanDocumentVersion(s.db.QueryRowContext(ctx, `
SELECT id, document_id, version_no, markdown_storage_key, schema_version, metadata_json, indexing_status, embedding_model, created_at
FROM document_versions
WHERE document_id = ?
ORDER BY version_no DESC
LIMIT 1
`, documentID))
}

func updateIngestionJobTx(ctx context.Context, tx *sql.Tx, id int64, status string, attempts int, code, message string) error {
	_, err := tx.ExecContext(ctx, `
UPDATE ingestion_jobs
SET status = ?, attempts = ?, error_code = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, attempts, code, message, id)
	return err
}

func updateDocumentStatusTx(ctx context.Context, tx *sql.Tx, id int64, status, code, message string) error {
	_, err := tx.ExecContext(ctx, `
UPDATE documents
SET status = ?, error_code = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, code, message, id)
	return err
}

func nextDocumentVersionNo(ctx context.Context, tx *sql.Tx, documentID int64) (int, error) {
	var current sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
SELECT MAX(version_no)
FROM document_versions
WHERE document_id = ?
`, documentID).Scan(&current); err != nil {
		return 0, err
	}
	if !current.Valid {
		return 1, nil
	}
	return int(current.Int64) + 1, nil
}
