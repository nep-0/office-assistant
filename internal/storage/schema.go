package storage

import (
	"context"
	"fmt"
)

func (s *Store) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`create table if not exists users (
			id text primary key,
			username text not null unique,
			password_hash text not null,
			role text not null,
			created_at text not null
		);`,
		`create table if not exists knowledge_bases (
			id text primary key,
			name text not null,
			created_by text not null,
			created_at text not null
		);`,
		`create table if not exists knowledge_base_memberships (
			knowledge_base_id text not null,
			user_id text not null,
			created_at text not null,
			primary key (knowledge_base_id, user_id)
		);`,
		`create table if not exists provider_settings (
			slot text primary key,
			kind text not null,
			base_url text not null,
			model text not null,
			api_key text not null,
			updated_at text not null
		);`,
		`create table if not exists documents (
			id text primary key,
			knowledge_base_id text not null,
			uploaded_by text not null,
			original_name text not null,
			storage_path text not null,
			content_type text not null,
			size_bytes integer not null,
			sha256 text not null,
			status text not null,
			status_reason text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists idx_documents_knowledge_base on documents (knowledge_base_id);`,
		`create unique index if not exists idx_documents_duplicate_upload on documents (knowledge_base_id, uploaded_by, sha256);`,
		`create table if not exists chunks (
			id text primary key,
			document_id text not null,
			knowledge_base_id text not null,
			content text not null,
			source_file_name text not null,
			page_number integer,
			chunk_index integer not null,
			content_type text not null,
			token_or_char_count integer not null,
			metadata_json text not null,
			created_at text not null
		);`,
		`create index if not exists idx_chunks_document on chunks (document_id);`,
		`create index if not exists idx_chunks_knowledge_base on chunks (knowledge_base_id);`,
		`create table if not exists chat_sessions (
			id text primary key,
			knowledge_base_id text not null,
			created_at text not null
		);`,
		`create table if not exists chat_messages (
			id text primary key,
			session_id text not null,
			role text not null,
			content text not null,
			created_at text not null
		);`,
		`create table if not exists citations (
			id text primary key,
			message_id text not null,
			document_id text not null,
			chunk_id text not null,
			source_file_name text not null,
			page_number integer,
			preview text not null,
			score real,
			created_at text not null
		);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute schema statement: %w", err)
		}
	}

	return nil
}
