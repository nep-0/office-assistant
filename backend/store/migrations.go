package store

import "database/sql"

func (s *Store) migrate() error {
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
	tool_calls_json TEXT NOT NULL DEFAULT '[]',
	tool_call_id TEXT NOT NULL DEFAULT '',
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
	if err := s.ensureColumn("document_versions", "embedding_model", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("chat_messages", "tool_calls_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	return s.ensureColumn("chat_messages", "tool_call_id", "TEXT NOT NULL DEFAULT ''")
}

func (s *Store) ensureColumn(table, column, definition string) error {
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
