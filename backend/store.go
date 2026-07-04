package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type store struct {
	db *sql.DB
}

type user struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type providerSetting struct {
	Purpose   string
	BaseURL   string
	Model     string
	APIKey    string
	UpdatedAt time.Time
}

type knowledgeBase struct {
	ID         int64
	OwnerID    int64
	OwnerName  string
	Name       string
	Visibility string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type documentRecord struct {
	ID               int64
	KnowledgeBaseID  int64
	OwnerID          int64
	OriginalFilename string
	DisplayName      string
	ContentType      string
	SizeBytes        int64
	SHA256           string
	StorageKey       string
	Status           string
	ErrorCode        string
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ingestionJob struct {
	ID           int64
	DocumentID   int64
	Status       string
	Attempts     int
	MaxAttempts  int
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type documentVersion struct {
	ID                 int64
	DocumentID         int64
	VersionNo          int
	MarkdownStorageKey string
	SchemaVersion      string
	MetadataJSON       string
	IndexingStatus     string
	EmbeddingModel     string
	CreatedAt          time.Time
}

type documentChunk struct {
	ID                int64
	DocumentID        int64
	DocumentVersionID int64
	ChunkNo           int
	Content           string
	HeadingPath       string
	SourceAnchorJSON  string
	TokenCount        int
	EmbeddingModel    string
	IndexingStatus    string
	CreatedAt         time.Time
}

type chatSession struct {
	ID              string
	UserID          int64
	KnowledgeBaseID int64
	Title           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type chatMessage struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Metadata  string
	CreatedAt time.Time
}

type retrievalChunk struct {
	ChunkID          int64
	DocumentID       int64
	DocumentName     string
	Content          string
	HeadingPath      string
	SourceAnchorJSON string
}

type activityEvent struct {
	ID         int64
	UserID     int64
	EventType  string
	EntityType string
	EntityID   string
	Details    string
	CreatedAt  time.Time
}

type workflowMetric struct {
	ID        int64
	Name      string
	ValueMS   int64
	Count     int64
	Details   string
	CreatedAt time.Time
}

type debugSetting struct {
	Enabled   bool
	Source    string
	UpdatedAt time.Time
}

func (s *store) Close() error {
	return s.db.Close()
}

func (s *store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS provider_settings (
	purpose TEXT PRIMARY KEY CHECK (purpose IN ('chat', 'embedding')),
	base_url TEXT NOT NULL,
	model TEXT NOT NULL,
	api_key TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS knowledge_bases (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	visibility TEXT NOT NULL CHECK (visibility IN ('private', 'public')) DEFAULT 'private',
	deleted_at TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS knowledge_bases_owner_idx ON knowledge_bases(owner_user_id);
CREATE INDEX IF NOT EXISTS knowledge_bases_visibility_idx ON knowledge_bases(visibility);

CREATE TABLE IF NOT EXISTS documents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	knowledge_base_id INTEGER NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
	owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	original_filename TEXT NOT NULL,
	display_name TEXT NOT NULL,
	content_type TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	sha256 TEXT NOT NULL,
	storage_key TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'uploaded',
	error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	deleted_at TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS documents_knowledge_base_idx ON documents(knowledge_base_id);
CREATE INDEX IF NOT EXISTS documents_hash_idx ON documents(knowledge_base_id, sha256);

CREATE TABLE IF NOT EXISTS ingestion_jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
	status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'cancel_requested', 'cancelled')),
	attempts INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 3,
	error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS ingestion_jobs_document_idx ON ingestion_jobs(document_id);
CREATE INDEX IF NOT EXISTS ingestion_jobs_status_idx ON ingestion_jobs(status);

CREATE TABLE IF NOT EXISTS document_versions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
	version_no INTEGER NOT NULL,
	markdown_storage_key TEXT NOT NULL,
	schema_version TEXT NOT NULL,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	indexing_status TEXT NOT NULL DEFAULT 'pending',
	embedding_model TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(document_id, version_no)
);

CREATE TABLE IF NOT EXISTS document_chunks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
	document_version_id INTEGER NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
	chunk_no INTEGER NOT NULL,
	content TEXT NOT NULL,
	heading_path TEXT NOT NULL DEFAULT '',
	source_anchor_json TEXT NOT NULL DEFAULT '{}',
	token_count INTEGER NOT NULL DEFAULT 0,
	embedding_model TEXT NOT NULL,
	indexing_status TEXT NOT NULL CHECK (indexing_status IN ('indexed', 'superseded', 'deleted')) DEFAULT 'indexed',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(document_version_id, chunk_no)
);

CREATE INDEX IF NOT EXISTS document_chunks_document_idx ON document_chunks(document_id);
CREATE INDEX IF NOT EXISTS document_chunks_version_idx ON document_chunks(document_version_id);
CREATE INDEX IF NOT EXISTS document_chunks_status_idx ON document_chunks(indexing_status);

CREATE VIRTUAL TABLE IF NOT EXISTS document_search_fts USING fts5(
	original_filename,
	display_name,
	content_type,
	status,
	created_at,
	extracted_text,
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	knowledge_base_id INTEGER NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
	title TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS chat_sessions_user_idx ON chat_sessions(user_id);
CREATE INDEX IF NOT EXISTS chat_sessions_knowledge_base_idx ON chat_sessions(knowledge_base_id);

CREATE TABLE IF NOT EXISTS chat_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
	role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool', 'error')),
	content TEXT NOT NULL,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS chat_messages_session_idx ON chat_messages(session_id, id);

CREATE TABLE IF NOT EXISTS activity_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL DEFAULT 0,
	event_type TEXT NOT NULL,
	entity_type TEXT NOT NULL DEFAULT '',
	entity_id TEXT NOT NULL DEFAULT '',
	details_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS activity_events_created_idx ON activity_events(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS activity_events_type_idx ON activity_events(event_type);

CREATE TABLE IF NOT EXISTS workflow_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	value_ms INTEGER NOT NULL DEFAULT 0,
	count INTEGER NOT NULL DEFAULT 0,
	details_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS workflow_metrics_created_idx ON workflow_metrics(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS workflow_metrics_name_idx ON workflow_metrics(name);

CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS debug_traces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	correlation_id TEXT NOT NULL,
	trace_type TEXT NOT NULL,
	payload_json TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS debug_traces_expires_idx ON debug_traces(expires_at);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("documents", "error_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("documents", "error_message", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("document_versions", "indexing_status", "TEXT NOT NULL DEFAULT 'pending'"); err != nil {
		return err
	}
	return s.ensureColumn("document_versions", "embedding_model", "TEXT NOT NULL DEFAULT ''")
}

func (s *store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func (s *store) countUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *store) createUser(ctx context.Context, username, passwordHash, role string) (user, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role)
VALUES (?, ?, ?)
`, username, passwordHash, role)
	if err != nil {
		return user{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return user{}, err
	}
	return s.findUserByID(ctx, id)
}

func (s *store) findUserByUsername(ctx context.Context, username string) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE username = ?
`, username))
}

func (s *store) findUserByID(ctx context.Context, id int64) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at
FROM users
WHERE id = ?
`, id))
}

func (s *store) createSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, expires_at)
VALUES (?, ?, ?)
`, id, userID, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *store) findUserBySession(ctx context.Context, sessionID string, now time.Time) (user, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
SELECT users.id, users.username, users.password_hash, users.role, users.created_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ? AND sessions.expires_at > ?
`, sessionID, now.UTC().Format(time.RFC3339)))
}

func (s *store) deleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

func (s *store) ensureProviderDefaults(ctx context.Context, defaults map[string]providerSetting) error {
	for purpose, setting := range defaults {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO provider_settings (purpose, base_url, model, api_key)
VALUES (?, ?, ?, ?)
ON CONFLICT(purpose) DO NOTHING
`, purpose, setting.BaseURL, setting.Model, setting.APIKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *store) listProviderSettings(ctx context.Context) ([]providerSetting, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
ORDER BY purpose
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []providerSetting
	for rows.Next() {
		setting, err := scanProviderSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *store) findProviderSetting(ctx context.Context, purpose string) (providerSetting, error) {
	return scanProviderSetting(s.db.QueryRowContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
WHERE purpose = ?
`, purpose))
}

func (s *store) updateProviderSetting(ctx context.Context, setting providerSetting) (providerSetting, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE provider_settings
SET base_url = ?, model = ?, api_key = ?, updated_at = CURRENT_TIMESTAMP
WHERE purpose = ?
`, setting.BaseURL, setting.Model, setting.APIKey, setting.Purpose)
	if err != nil {
		return providerSetting{}, err
	}
	return s.findProviderSetting(ctx, setting.Purpose)
}

func (s *store) createKnowledgeBase(ctx context.Context, ownerID int64, name, visibility string) (knowledgeBase, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO knowledge_bases (owner_user_id, name, visibility)
VALUES (?, ?, ?)
`, ownerID, name, visibility)
	if err != nil {
		return knowledgeBase{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return knowledgeBase{}, err
	}
	return s.findKnowledgeBaseByID(ctx, id)
}

func (s *store) listKnowledgeBasesForUser(ctx context.Context, current user) ([]knowledgeBase, error) {
	query := `
SELECT knowledge_bases.id, knowledge_bases.owner_user_id, users.username, knowledge_bases.name, knowledge_bases.visibility, knowledge_bases.created_at, knowledge_bases.updated_at
FROM knowledge_bases
JOIN users ON users.id = knowledge_bases.owner_user_id
WHERE knowledge_bases.deleted_at IS NULL
`
	args := []any{}
	if current.Role != roleAdmin {
		query += ` AND (knowledge_bases.owner_user_id = ? OR knowledge_bases.visibility = 'public')`
		args = append(args, current.ID)
	}
	query += ` ORDER BY knowledge_bases.updated_at DESC, knowledge_bases.id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bases []knowledgeBase
	for rows.Next() {
		kb, err := scanKnowledgeBase(rows)
		if err != nil {
			return nil, err
		}
		bases = append(bases, kb)
	}
	return bases, rows.Err()
}

func (s *store) findKnowledgeBaseByID(ctx context.Context, id int64) (knowledgeBase, error) {
	return scanKnowledgeBase(s.db.QueryRowContext(ctx, `
SELECT knowledge_bases.id, knowledge_bases.owner_user_id, users.username, knowledge_bases.name, knowledge_bases.visibility, knowledge_bases.created_at, knowledge_bases.updated_at
FROM knowledge_bases
JOIN users ON users.id = knowledge_bases.owner_user_id
WHERE knowledge_bases.id = ? AND knowledge_bases.deleted_at IS NULL
`, id))
}

func (s *store) updateKnowledgeBase(ctx context.Context, id int64, name, visibility string) (knowledgeBase, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases
SET name = ?, visibility = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
`, name, visibility, id)
	if err != nil {
		return knowledgeBase{}, err
	}
	return s.findKnowledgeBaseByID(ctx, id)
}

func (s *store) deleteKnowledgeBase(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
`, id)
	return err
}

func (s *store) createDocument(ctx context.Context, doc documentRecord) (documentRecord, error) {
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
		return documentRecord{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return documentRecord{}, err
	}
	return s.findDocumentByID(ctx, id)
}

func (s *store) listDocumentsForKnowledgeBase(ctx context.Context, knowledgeBaseID int64) ([]documentRecord, error) {
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

	var docs []documentRecord
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *store) findDocumentDuplicateInKnowledgeBase(ctx context.Context, knowledgeBaseID int64, hash string) (documentRecord, error) {
	return scanDocument(s.db.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE knowledge_base_id = ? AND sha256 = ? AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1
`, knowledgeBaseID, hash))
}

func (s *store) findDocumentByID(ctx context.Context, id int64) (documentRecord, error) {
	return scanDocument(s.db.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE id = ? AND deleted_at IS NULL
`, id))
}

func (s *store) createIngestionJob(ctx context.Context, documentID int64) (ingestionJob, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO ingestion_jobs (document_id, status)
VALUES (?, 'pending')
`, documentID)
	if err != nil {
		return ingestionJob{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ingestionJob{}, err
	}
	return s.findIngestionJobByID(ctx, id)
}

func (s *store) findLatestIngestionJobForDocument(ctx context.Context, documentID int64) (ingestionJob, error) {
	return scanIngestionJob(s.db.QueryRowContext(ctx, `
SELECT id, document_id, status, attempts, max_attempts, error_code, error_message, created_at, updated_at
FROM ingestion_jobs
WHERE document_id = ?
ORDER BY id DESC
LIMIT 1
`, documentID))
}

func (s *store) findIngestionJobByID(ctx context.Context, id int64) (ingestionJob, error) {
	return scanIngestionJob(s.db.QueryRowContext(ctx, `
SELECT id, document_id, status, attempts, max_attempts, error_code, error_message, created_at, updated_at
FROM ingestion_jobs
WHERE id = ?
`, id))
}

func (s *store) claimNextIngestionJob(ctx context.Context) (ingestionJob, documentRecord, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ingestionJob{}, documentRecord{}, false, err
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
			return ingestionJob{}, documentRecord{}, false, nil
		}
		return ingestionJob{}, documentRecord{}, false, err
	}

	if job.Status == ingestionJobCancelRequested {
		if err := updateIngestionJobTx(ctx, tx, job.ID, ingestionJobCancelled, job.Attempts, "cancelled", "ingestion cancelled"); err != nil {
			return ingestionJob{}, documentRecord{}, false, err
		}
		if err := updateDocumentStatusTx(ctx, tx, job.DocumentID, documentStatusCancelled, "cancelled", "ingestion cancelled"); err != nil {
			return ingestionJob{}, documentRecord{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return ingestionJob{}, documentRecord{}, false, err
		}
		return ingestionJob{}, documentRecord{}, false, nil
	}

	attempts := job.Attempts + 1
	if _, err := tx.ExecContext(ctx, `
UPDATE ingestion_jobs
SET status = 'processing', attempts = ?, error_code = '', error_message = '', updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, attempts, job.ID); err != nil {
		return ingestionJob{}, documentRecord{}, false, err
	}
	if err := updateDocumentStatusTx(ctx, tx, job.DocumentID, documentStatusProcessing, "", ""); err != nil {
		return ingestionJob{}, documentRecord{}, false, err
	}
	doc, err := scanDocument(tx.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, error_code, error_message, created_at, updated_at
FROM documents
WHERE id = ? AND deleted_at IS NULL
`, job.DocumentID))
	if err != nil {
		return ingestionJob{}, documentRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ingestionJob{}, documentRecord{}, false, err
	}
	job.Status = ingestionJobProcessing
	job.Attempts = attempts
	return job, doc, true, nil
}

func (s *store) completeIngestionJob(ctx context.Context, job ingestionJob, doc documentRecord, version documentVersion, chunks []documentChunk) error {
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

func (s *store) reprocessDocument(ctx context.Context, documentID int64) (ingestionJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ingestionJob{}, err
	}
	defer tx.Rollback()
	if err := updateDocumentStatusTx(ctx, tx, documentID, documentStatusPending, "", ""); err != nil {
		return ingestionJob{}, err
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO ingestion_jobs (document_id, status)
VALUES (?, 'pending')
`, documentID)
	if err != nil {
		return ingestionJob{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ingestionJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return ingestionJob{}, err
	}
	return s.findIngestionJobByID(ctx, id)
}

func (s *store) deleteDocument(ctx context.Context, documentID int64) error {
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

func (s *store) failIngestionJob(ctx context.Context, job ingestionJob, code, message string) error {
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

func (s *store) cancelIngestionForDocument(ctx context.Context, documentID int64) error {
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

func (s *store) findLatestDocumentVersion(ctx context.Context, documentID int64) (documentVersion, error) {
	return scanDocumentVersion(s.db.QueryRowContext(ctx, `
SELECT id, document_id, version_no, markdown_storage_key, schema_version, metadata_json, indexing_status, embedding_model, created_at
FROM document_versions
WHERE document_id = ?
ORDER BY version_no DESC
LIMIT 1
`, documentID))
}

type documentSearchFilter struct {
	Query       string
	Status      string
	ContentType string
	DateFrom    string
	DateTo      string
}

func (s *store) searchDocuments(ctx context.Context, knowledgeBaseID int64, filter documentSearchFilter) ([]documentRecord, error) {
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
	var docs []documentRecord
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *store) listIndexedChunks(ctx context.Context) ([]documentChunk, error) {
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
	var chunks []documentChunk
	for rows.Next() {
		chunk, err := scanDocumentChunk(rows)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *store) findRetrievalChunk(ctx context.Context, chunkID, knowledgeBaseID int64) (retrievalChunk, error) {
	var chunk retrievalChunk
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

func (s *store) createChatSession(ctx context.Context, session chatSession) (chatSession, error) {
	if session.ID == "" {
		return chatSession{}, errors.New("chat session id is required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_sessions (id, user_id, knowledge_base_id, title)
VALUES (?, ?, ?, ?)
`, session.ID, session.UserID, session.KnowledgeBaseID, session.Title)
	if err != nil {
		return chatSession{}, err
	}
	return s.findChatSession(ctx, session.ID)
}

func (s *store) findChatSession(ctx context.Context, id string) (chatSession, error) {
	return scanChatSession(s.db.QueryRowContext(ctx, `
SELECT id, user_id, knowledge_base_id, title, created_at, updated_at
FROM chat_sessions
WHERE id = ?
`, id))
}

func (s *store) appendChatMessage(ctx context.Context, msg chatMessage) error {
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

func (s *store) listChatMessages(ctx context.Context, sessionID string, limit int) ([]chatMessage, error) {
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
	var messages []chatMessage
	for rows.Next() {
		msg, err := scanChatMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *store) appendActivity(ctx context.Context, userID int64, eventType, entityType, entityID string, details map[string]any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO activity_events (user_id, event_type, entity_type, entity_id, details_json)
VALUES (?, ?, ?, ?, ?)
`, userID, eventType, entityType, entityID, string(data))
	return err
}

func (s *store) listActivity(ctx context.Context, limit int) ([]activityEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, event_type, entity_type, entity_id, details_json, created_at
FROM activity_events
ORDER BY id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []activityEvent
	for rows.Next() {
		event, err := scanActivityEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *store) recordMetric(ctx context.Context, name string, value time.Duration, count int64, details map[string]any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO workflow_metrics (name, value_ms, count, details_json)
VALUES (?, ?, ?, ?)
`, name, value.Milliseconds(), count, string(data))
	return err
}

func (s *store) listMetrics(ctx context.Context, limit int) ([]workflowMetric, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, value_ms, count, details_json, created_at
FROM workflow_metrics
ORDER BY id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []workflowMetric
	for rows.Next() {
		metric, err := scanWorkflowMetric(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, rows.Err()
}

func (s *store) debugSetting(ctx context.Context, envEnabled bool) (debugSetting, error) {
	if envEnabled {
		return debugSetting{Enabled: true, Source: "environment", UpdatedAt: time.Now().UTC()}, nil
	}
	var value string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT value, updated_at
FROM app_settings
WHERE key = 'debug_mode'
`).Scan(&value, &updatedAt)
	if err != nil {
		if notFound(err) {
			return debugSetting{Enabled: false, Source: "default", UpdatedAt: time.Time{}}, nil
		}
		return debugSetting{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return debugSetting{}, err
	}
	return debugSetting{Enabled: value == "true", Source: "admin", UpdatedAt: updated}, nil
}

func (s *store) setDebugMode(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('debug_mode', ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
`, value)
	return err
}

func (s *store) appendDebugTrace(ctx context.Context, correlationID, traceType string, payload map[string]any, retention time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(retention).Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO debug_traces (correlation_id, trace_type, payload_json, expires_at)
VALUES (?, ?, ?, ?)
`, correlationID, traceType, string(data), expiresAt)
	return err
}

func (s *store) pruneDebugTraces(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM debug_traces WHERE expires_at <= ?`, now.UTC().Format(time.RFC3339))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (user, error) {
	var u user
	var createdAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt); err != nil {
		return user{}, err
	}
	parsed, err := parseSQLiteTime(createdAt)
	if err != nil {
		return user{}, err
	}
	u.CreatedAt = parsed
	return u, nil
}

func scanProviderSetting(row rowScanner) (providerSetting, error) {
	var setting providerSetting
	var updatedAt string
	if err := row.Scan(&setting.Purpose, &setting.BaseURL, &setting.Model, &setting.APIKey, &updatedAt); err != nil {
		return providerSetting{}, err
	}
	parsed, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return providerSetting{}, err
	}
	setting.UpdatedAt = parsed
	return setting, nil
}

func scanKnowledgeBase(row rowScanner) (knowledgeBase, error) {
	var kb knowledgeBase
	var createdAt string
	var updatedAt string
	if err := row.Scan(&kb.ID, &kb.OwnerID, &kb.OwnerName, &kb.Name, &kb.Visibility, &createdAt, &updatedAt); err != nil {
		return knowledgeBase{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return knowledgeBase{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return knowledgeBase{}, err
	}
	kb.CreatedAt = created
	kb.UpdatedAt = updated
	return kb, nil
}

func scanDocument(row rowScanner) (documentRecord, error) {
	var doc documentRecord
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
		return documentRecord{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return documentRecord{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return documentRecord{}, err
	}
	doc.CreatedAt = created
	doc.UpdatedAt = updated
	return doc, nil
}

func scanIngestionJob(row rowScanner) (ingestionJob, error) {
	var job ingestionJob
	var createdAt string
	var updatedAt string
	if err := row.Scan(&job.ID, &job.DocumentID, &job.Status, &job.Attempts, &job.MaxAttempts, &job.ErrorCode, &job.ErrorMessage, &createdAt, &updatedAt); err != nil {
		return ingestionJob{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return ingestionJob{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return ingestionJob{}, err
	}
	job.CreatedAt = created
	job.UpdatedAt = updated
	return job, nil
}

func scanDocumentVersion(row rowScanner) (documentVersion, error) {
	var version documentVersion
	var createdAt string
	if err := row.Scan(&version.ID, &version.DocumentID, &version.VersionNo, &version.MarkdownStorageKey, &version.SchemaVersion, &version.MetadataJSON, &version.IndexingStatus, &version.EmbeddingModel, &createdAt); err != nil {
		return documentVersion{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return documentVersion{}, err
	}
	version.CreatedAt = created
	return version, nil
}

func scanDocumentChunk(row rowScanner) (documentChunk, error) {
	var chunk documentChunk
	var createdAt string
	if err := row.Scan(&chunk.ID, &chunk.DocumentID, &chunk.DocumentVersionID, &chunk.ChunkNo, &chunk.Content, &chunk.HeadingPath, &chunk.SourceAnchorJSON, &chunk.TokenCount, &chunk.EmbeddingModel, &chunk.IndexingStatus, &createdAt); err != nil {
		return documentChunk{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return documentChunk{}, err
	}
	chunk.CreatedAt = created
	return chunk, nil
}

func scanChatSession(row rowScanner) (chatSession, error) {
	var session chatSession
	var createdAt string
	var updatedAt string
	if err := row.Scan(&session.ID, &session.UserID, &session.KnowledgeBaseID, &session.Title, &createdAt, &updatedAt); err != nil {
		return chatSession{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return chatSession{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return chatSession{}, err
	}
	session.CreatedAt = created
	session.UpdatedAt = updated
	return session, nil
}

func scanChatMessage(row rowScanner) (chatMessage, error) {
	var msg chatMessage
	var createdAt string
	if err := row.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Metadata, &createdAt); err != nil {
		return chatMessage{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return chatMessage{}, err
	}
	msg.CreatedAt = created
	return msg, nil
}

func scanActivityEvent(row rowScanner) (activityEvent, error) {
	var event activityEvent
	var createdAt string
	if err := row.Scan(&event.ID, &event.UserID, &event.EventType, &event.EntityType, &event.EntityID, &event.Details, &createdAt); err != nil {
		return activityEvent{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return activityEvent{}, err
	}
	event.CreatedAt = created
	return event, nil
}

func scanWorkflowMetric(row rowScanner) (workflowMetric, error) {
	var metric workflowMetric
	var createdAt string
	if err := row.Scan(&metric.ID, &metric.Name, &metric.ValueMS, &metric.Count, &metric.Details, &createdAt); err != nil {
		return workflowMetric{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return workflowMetric{}, err
	}
	metric.CreatedAt = created
	return metric, nil
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

func upsertDocumentSearchTx(ctx context.Context, tx *sql.Tx, doc documentRecord, markdownStorageKey string) error {
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
