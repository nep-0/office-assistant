package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"office-assistant/internal/auth"
	"office-assistant/internal/document"
	"office-assistant/internal/storage"
)

func (a *api) listDocuments(w http.ResponseWriter, r *http.Request) {
	knowledgeBaseID := r.PathValue("id")
	if ok := a.requireKnowledgeBaseAccess(w, r, knowledgeBaseID); !ok {
		return
	}

	docs, err := a.store.ListDocuments(r.Context(), knowledgeBaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (a *api) uploadDocument(w http.ResponseWriter, r *http.Request) {
	knowledgeBaseID := r.PathValue("id")
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("auth claims missing"))
		return
	}
	if ok := a.requireKnowledgeBaseAccess(w, r, knowledgeBaseID); !ok {
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse multipart form: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("file field is required: %w", err))
		return
	}
	defer file.Close()

	documentID := newID("doc")
	storagePath := filepath.Join(a.uploadDir, knowledgeBaseID, documentID, filepath.Base(header.Filename))
	sha, size, err := saveUploadedFile(storagePath, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	doc, err := a.store.CreateDocument(r.Context(), storage.Document{
		ID:              documentID,
		KnowledgeBaseID: knowledgeBaseID,
		UploadedBy:      claims.UserID,
		OriginalName:    header.Filename,
		StoragePath:     storagePath,
		ContentType:     contentType(header),
		SizeBytes:       size,
		SHA256:          sha,
		Status:          "pending",
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	go a.processDocument(doc)
	writeJSON(w, http.StatusAccepted, doc)
}

func (a *api) processDocument(doc storage.Document) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := a.store.UpdateDocumentStatus(ctx, doc.ID, "processing", ""); err != nil {
		a.logger.Warn("mark document processing failed", "document_id", doc.ID, "error", err)
		return
	}

	chunks, err := a.processor.Process(ctx, document.ProcessInput{
		DocumentID:      doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		FilePath:        doc.StoragePath,
		OriginalName:    doc.OriginalName,
		ContentType:     doc.ContentType,
	})
	if err != nil {
		a.logger.Warn("document processing failed", "document_id", doc.ID, "error", err)
		_ = a.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}

	storedChunks, err := normalizeChunks(doc, chunks)
	if err != nil {
		a.logger.Warn("normalize chunks failed", "document_id", doc.ID, "error", err)
		_ = a.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}
	if err := a.store.ReplaceDocumentChunks(ctx, doc.ID, storedChunks); err != nil {
		a.logger.Warn("store chunks failed", "document_id", doc.ID, "error", err)
		_ = a.store.UpdateDocumentStatus(ctx, doc.ID, "failed", err.Error())
		return
	}
	if err := a.store.UpdateDocumentStatus(ctx, doc.ID, "indexed", ""); err != nil {
		a.logger.Warn("mark document indexed failed", "document_id", doc.ID, "error", err)
	}
}

func (a *api) requireKnowledgeBaseAccess(w http.ResponseWriter, r *http.Request, knowledgeBaseID string) bool {
	claims, _ := auth.ClaimsFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("user no longer exists"))
		return false
	}
	allowed, err := a.store.CanAccessKnowledgeBase(r.Context(), user, knowledgeBaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, errors.New("knowledge base access is required"))
		return false
	}
	return true
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

func contentType(header *multipart.FileHeader) string {
	if values := header.Header.Values("Content-Type"); len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	return "application/octet-stream"
}

func normalizeChunks(doc storage.Document, chunks []document.Chunk) ([]storage.Chunk, error) {
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
			chunkID = newID("chunk")
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
