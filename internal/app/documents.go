package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"office-assistant/internal/document"
	"office-assistant/internal/storage"
	"office-assistant/internal/utils"
)

type DocumentStore interface {
	GetUserByID(ctx context.Context, id string) (storage.User, error)
	CanAccessKnowledgeBase(ctx context.Context, user storage.User, knowledgeBaseID string) (bool, error)
	CreateDocument(ctx context.Context, doc storage.Document) (storage.Document, error)
	ListDocuments(ctx context.Context, knowledgeBaseID string) ([]storage.Document, error)
	UpdateDocumentStatus(ctx context.Context, documentID, status, reason string) error
	ReplaceDocumentChunks(ctx context.Context, documentID string, chunks []storage.Chunk) error
}

type DocumentService struct {
	store     DocumentStore
	processor document.Processor
	uploadDir string
	logger    *slog.Logger
}

type DocumentServiceOptions struct {
	Store     DocumentStore
	Processor document.Processor
	UploadDir string
	Logger    *slog.Logger
}

type UploadInput struct {
	KnowledgeBaseID string
	UserID          string
	OriginalName    string
	ContentType     string
	File            multipart.File
}

func NewDocumentService(options DocumentServiceOptions) *DocumentService {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	uploadDir := options.UploadDir
	if uploadDir == "" {
		uploadDir = "data/uploads"
	}
	return &DocumentService{
		store:     options.Store,
		processor: options.Processor,
		uploadDir: uploadDir,
		logger:    logger,
	}
}

func (s *DocumentService) List(ctx context.Context, userID, knowledgeBaseID string) ([]storage.Document, error) {
	if err := s.requireAccess(ctx, userID, knowledgeBaseID); err != nil {
		return nil, err
	}
	return s.store.ListDocuments(ctx, knowledgeBaseID)
}

func (s *DocumentService) Upload(ctx context.Context, input UploadInput) (storage.Document, error) {
	if err := s.requireAccess(ctx, input.UserID, input.KnowledgeBaseID); err != nil {
		return storage.Document{}, err
	}

	documentID := utils.NewID("doc")
	storagePath := filepath.Join(s.uploadDir, input.KnowledgeBaseID, documentID, filepath.Base(input.OriginalName))
	sha, size, err := saveUploadedFile(storagePath, input.File)
	if err != nil {
		return storage.Document{}, err
	}

	doc, err := s.store.CreateDocument(ctx, storage.Document{
		ID:              documentID,
		KnowledgeBaseID: input.KnowledgeBaseID,
		UploadedBy:      input.UserID,
		OriginalName:    input.OriginalName,
		StoragePath:     storagePath,
		ContentType:     input.ContentType,
		SizeBytes:       size,
		SHA256:          sha,
		Status:          "pending",
	})
	if err != nil {
		return storage.Document{}, err
	}

	go s.process(doc)
	return doc, nil
}

func (s *DocumentService) process(doc storage.Document) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := s.store.UpdateDocumentStatus(ctx, doc.ID, "processing", ""); err != nil {
		s.logger.Warn("mark document processing failed", "document_id", doc.ID, "error", err)
		return
	}

	chunks, err := s.processor.Process(ctx, document.ProcessInput{
		DocumentID:      doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		FilePath:        doc.StoragePath,
		OriginalName:    doc.OriginalName,
		ContentType:     doc.ContentType,
	})
	if err != nil {
		s.logger.Warn("document processing failed", "document_id", doc.ID, "error", err)
		_ = s.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}

	storedChunks, err := normalizeChunks(doc, chunks)
	if err != nil {
		s.logger.Warn("normalize chunks failed", "document_id", doc.ID, "error", err)
		_ = s.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}
	if err := s.store.ReplaceDocumentChunks(ctx, doc.ID, storedChunks); err != nil {
		s.logger.Warn("store chunks failed", "document_id", doc.ID, "error", err)
		_ = s.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}
	if err := s.store.UpdateDocumentStatus(ctx, doc.ID, "indexed", ""); err != nil {
		s.logger.Warn("mark document indexed failed", "document_id", doc.ID, "error", err)
	}
}

func (s *DocumentService) requireAccess(ctx context.Context, userID, knowledgeBaseID string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return PermissionError{Status: "unauthorized", Message: "user no longer exists"}
	}
	allowed, err := s.store.CanAccessKnowledgeBase(ctx, user, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("check knowledge base access: %w", err)
	}
	if !allowed {
		return PermissionError{Status: "forbidden", Message: "knowledge base access is required"}
	}
	return nil
}

type PermissionError struct {
	Status  string
	Message string
}

func (e PermissionError) Error() string {
	return e.Message
}

func saveUploadedFile(path string, file multipart.File) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", 0, fmt.Errorf("create upload directory: %w", err)
	}
	target, err := os.Create(path)
	if err != nil {
		return "", 0, fmt.Errorf("create upload file: %w", err)
	}
	defer target.Close()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(target, hasher), file)
	if err != nil {
		return "", 0, fmt.Errorf("save upload file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func normalizeChunks(doc storage.Document, chunks []document.Chunk) ([]storage.Chunk, error) {
	if len(chunks) == 0 {
		return nil, errors.New("processor returned no chunks")
	}
	stored := make([]storage.Chunk, 0, len(chunks))
	for i, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			return nil, fmt.Errorf("chunk %d content is required", i)
		}
		metadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal chunk metadata: %w", err)
		}
		chunkID := chunk.ID
		if chunkID == "" {
			chunkID = utils.NewID("chunk")
		}
		contentType := chunk.ContentType
		if contentType == "" {
			contentType = "text"
		}
		sourceFileName := chunk.SourceFileName
		if sourceFileName == "" {
			sourceFileName = doc.OriginalName
		}
		stored = append(stored, storage.Chunk{
			ID:               chunkID,
			DocumentID:       doc.ID,
			KnowledgeBaseID:  doc.KnowledgeBaseID,
			Content:          chunk.Content,
			SourceFileName:   sourceFileName,
			PageNumber:       chunk.PageNumber,
			ChunkIndex:       chunk.ChunkIndex,
			ContentType:      contentType,
			TokenOrCharCount: chunk.TokenOrCharCount,
			MetadataJSON:     string(metadata),
		})
	}
	return stored, nil
}
