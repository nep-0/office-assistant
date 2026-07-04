package main

import (
	"context"
	"database/sql"
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
	CreatedAt          time.Time
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
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(document_id, version_no)
);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("documents", "error_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return s.ensureColumn("documents", "error_message", "TEXT NOT NULL DEFAULT ''")
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

func (s *store) completeIngestionJob(ctx context.Context, job ingestionJob, doc documentRecord, version documentVersion) error {
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
INSERT INTO document_versions (document_id, version_no, markdown_storage_key, schema_version, metadata_json)
VALUES (?, ?, ?, ?, ?)
`, doc.ID, versionNo, version.MarkdownStorageKey, version.SchemaVersion, version.MetadataJSON); err != nil {
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
SELECT id, document_id, version_no, markdown_storage_key, schema_version, metadata_json, created_at
FROM document_versions
WHERE document_id = ?
ORDER BY version_no DESC
LIMIT 1
`, documentID))
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
	if err := row.Scan(&version.ID, &version.DocumentID, &version.VersionNo, &version.MarkdownStorageKey, &version.SchemaVersion, &version.MetadataJSON, &createdAt); err != nil {
		return documentVersion{}, err
	}
	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return documentVersion{}, err
	}
	version.CreatedAt = created
	return version, nil
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

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
