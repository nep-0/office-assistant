package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"office-assistant/backend/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const (
	roleAdmin = "admin"

	documentStatusPending    = "pending"
	documentStatusProcessing = "processing"
	documentStatusReady      = "ready"
	documentStatusFailed     = "failed"
	documentStatusCancelled  = "cancelled"

	ingestionJobPending         = "pending"
	ingestionJobProcessing      = "processing"
	ingestionJobSucceeded       = "succeeded"
	ingestionJobFailed          = "failed"
	ingestionJobCancelRequested = "cancel_requested"
	ingestionJobCancelled       = "cancelled"
)

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func notFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

type User = domain.User
type ProviderSetting = domain.ProviderSetting
type KnowledgeBase = domain.KnowledgeBase
type DocumentRecord = domain.DocumentRecord
type IngestionJob = domain.IngestionJob
type DocumentVersion = domain.DocumentVersion
type DocumentChunk = domain.DocumentChunk
type ChatSession = domain.ChatSession
type ChatMessage = domain.ChatMessage
type RetrievalChunk = domain.RetrievalChunk
type ActivityEvent = domain.ActivityEvent
type WorkflowMetric = domain.WorkflowMetric
type DebugSetting = domain.DebugSetting

func (s *Store) Close() error {
	return s.db.Close()
}
