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
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
	deleted_at TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS documents_knowledge_base_idx ON documents(knowledge_base_id);
CREATE INDEX IF NOT EXISTS documents_hash_idx ON documents(knowledge_base_id, sha256);
`)
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
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, created_at, updated_at
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
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, created_at, updated_at
FROM documents
WHERE knowledge_base_id = ? AND sha256 = ? AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1
`, knowledgeBaseID, hash))
}

func (s *store) findDocumentByID(ctx context.Context, id int64) (documentRecord, error) {
	return scanDocument(s.db.QueryRowContext(ctx, `
SELECT id, knowledge_base_id, owner_user_id, original_filename, display_name, content_type, size_bytes, sha256, storage_key, status, created_at, updated_at
FROM documents
WHERE id = ? AND deleted_at IS NULL
`, id))
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

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
