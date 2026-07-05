package documents

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"office-assistant/backend/domain"
	"office-assistant/backend/ingestion"
	"office-assistant/backend/knowledge"
	"office-assistant/backend/store"
	"office-assistant/backend/utils"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

type LifecycleError struct {
	Code    string
	Message string
}

func (err LifecycleError) Error() string {
	return err.Code
}

type DuplicateError struct {
	Duplicate domain.DocumentRecord
}

func (err DuplicateError) Error() string {
	return "duplicate_document"
}

type Storage interface {
	WriteUploadTemp(file io.Reader) (string, string, int64, error)
	PrepareStoragePath() (string, string, error)
	MoveTemp(tempPath, finalPath string) error
	Remove(path string) error
	WriteExtractedMarkdown(doc domain.DocumentRecord, markdown string) (string, error)
	Read(storageKey string) ([]byte, error)
}

type Extractor interface {
	Extract(ctx context.Context, doc domain.DocumentRecord) (ingestion.ExtractionPackage, error)
}

type RetrievalIndex interface {
	Preflight(ctx context.Context, chunks []domain.DocumentChunk) error
	Refresh(ctx context.Context) error
}

type MetricRecorder func(context.Context, string, time.Duration, int64, map[string]any) error

type Lifecycle struct {
	Store            *store.Store
	Storage          Storage
	Extractor        Extractor
	ChunkingStrategy ingestion.ChunkingStrategy
	RetrievalIndex   RetrievalIndex
	EmbeddingPurpose string
	RecordMetric     MetricRecorder
}

type UploadInput struct {
	Current          domain.User
	KnowledgeBase    domain.KnowledgeBase
	File             io.Reader
	OriginalFilename string
	ContentType      string
	ConfirmDuplicate bool
}

func (l Lifecycle) Upload(ctx context.Context, in UploadInput) (domain.DocumentRecord, error) {
	if !knowledge.CanModify(in.Current, in.KnowledgeBase) {
		return domain.DocumentRecord{}, LifecycleError{Code: "forbidden", Message: "knowledge base write access required"}
	}
	originalFilename := CleanFilename(in.OriginalFilename)
	if !SupportedOfficeInput(originalFilename) {
		return domain.DocumentRecord{}, LifecycleError{Code: "unsupported_office_input", Message: "file type is not supported"}
	}

	tempPath, hash, size, err := l.Storage.WriteUploadTemp(in.File)
	if err != nil {
		return domain.DocumentRecord{}, err
	}
	defer l.Storage.Remove(tempPath)

	duplicate, err := l.Store.FindDocumentDuplicateInKnowledgeBase(ctx, in.KnowledgeBase.ID, hash)
	if err == nil && !in.ConfirmDuplicate {
		return domain.DocumentRecord{}, DuplicateError{Duplicate: duplicate}
	}
	if err != nil && !utils.NotFound(err) {
		return domain.DocumentRecord{}, err
	}

	storageKey, finalPath, err := l.Storage.PrepareStoragePath()
	if err != nil {
		return domain.DocumentRecord{}, err
	}
	if err := l.Storage.MoveTemp(tempPath, finalPath); err != nil {
		return domain.DocumentRecord{}, err
	}

	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	doc, err := l.Store.CreateDocument(ctx, domain.DocumentRecord{
		KnowledgeBaseID:  in.KnowledgeBase.ID,
		OwnerID:          in.Current.ID,
		OriginalFilename: originalFilename,
		DisplayName:      originalFilename,
		ContentType:      contentType,
		SizeBytes:        size,
		SHA256:           hash,
		StorageKey:       storageKey,
		Status:           StatusPending,
	})
	if err != nil {
		_ = l.Storage.Remove(finalPath)
		return domain.DocumentRecord{}, err
	}
	if _, err := l.Store.CreateIngestionJob(ctx, doc.ID); err != nil {
		return domain.DocumentRecord{}, err
	}
	_ = l.Store.AppendActivity(ctx, in.Current.ID, "document_uploaded", "document", formatID(doc.ID), map[string]any{"knowledge_base_id": in.KnowledgeBase.ID, "filename": doc.DisplayName})
	return doc, nil
}

func (l Lifecycle) Delete(ctx context.Context, current domain.User, doc domain.DocumentRecord) error {
	kb, err := l.Store.FindKnowledgeBaseByID(ctx, doc.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if !knowledge.CanModify(current, kb) {
		return LifecycleError{Code: "forbidden", Message: "knowledge base write access required"}
	}
	if err := l.Store.DeleteDocument(ctx, doc.ID); err != nil {
		return err
	}
	if l.RetrievalIndex != nil {
		if err := l.RetrievalIndex.Refresh(ctx); err != nil {
			return err
		}
	}
	_ = l.Store.AppendActivity(ctx, current.ID, "document_deleted", "document", formatID(doc.ID), map[string]any{"knowledge_base_id": kb.ID, "filename": doc.DisplayName})
	return nil
}

func (l Lifecycle) Reprocess(ctx context.Context, current domain.User, doc domain.DocumentRecord) error {
	kb, err := l.Store.FindKnowledgeBaseByID(ctx, doc.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if !knowledge.CanModify(current, kb) {
		return LifecycleError{Code: "forbidden", Message: "knowledge base write access required"}
	}
	if _, err := l.Store.ReprocessDocument(ctx, doc.ID); err != nil {
		return err
	}
	_ = l.Store.AppendActivity(ctx, current.ID, "document_reprocess_requested", "document", formatID(doc.ID), map[string]any{"knowledge_base_id": kb.ID, "filename": doc.DisplayName})
	return nil
}

func (l Lifecycle) CancelIngestion(ctx context.Context, current domain.User, doc domain.DocumentRecord) error {
	kb, err := l.Store.FindKnowledgeBaseByID(ctx, doc.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if !knowledge.CanModify(current, kb) {
		return LifecycleError{Code: "forbidden", Message: "knowledge base write access required"}
	}
	return l.Store.CancelIngestionForDocument(ctx, doc.ID)
}

func (l Lifecycle) ExtractedMarkdown(ctx context.Context, current domain.User, doc domain.DocumentRecord) (string, error) {
	kb, err := l.Store.FindKnowledgeBaseByID(ctx, doc.KnowledgeBaseID)
	if err != nil {
		return "", err
	}
	if !knowledge.CanRead(current, kb) {
		return "", LifecycleError{Code: "document_not_found", Message: "document not found"}
	}
	version, err := l.Store.FindLatestDocumentVersion(ctx, doc.ID)
	if err != nil {
		return "", err
	}
	data, err := l.Storage.Read(version.MarkdownStorageKey)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (l Lifecycle) ProcessNextIngestionJob(ctx context.Context) (bool, error) {
	started := time.Now()
	job, doc, ok, err := l.Store.ClaimNextIngestionJob(ctx)
	if err != nil || !ok {
		return ok, err
	}

	pkg, err := l.Extractor.Extract(ctx, doc)
	if err != nil {
		return true, l.Store.FailIngestionJob(ctx, job, "document_extraction_failed", err.Error())
	}
	if pkg.Markdown == "" {
		return true, l.Store.FailIngestionJob(ctx, job, "empty_extraction", "document extraction returned no Markdown")
	}

	metadata, err := json.Marshal(map[string]any{
		"metadata":       pkg.Metadata,
		"warnings":       pkg.Warnings,
		"ocr":            pkg.OCR,
		"source_anchors": pkg.SourceAnchors,
	})
	if err != nil {
		return true, l.Store.FailIngestionJob(ctx, job, "metadata_encode_failed", err.Error())
	}

	markdownKey, err := l.Storage.WriteExtractedMarkdown(doc, pkg.Markdown)
	if err != nil {
		return true, l.Store.FailIngestionJob(ctx, job, "artifact_store_failed", err.Error())
	}
	chunker := l.ChunkingStrategy
	if chunker == nil {
		chunker = ingestion.MarkdownChunkingStrategy{}
	}
	chunks, err := chunker.Chunk(pkg.Markdown, pkg.SourceAnchors)
	if err != nil {
		return true, l.Store.FailIngestionJob(ctx, job, "chunking_failed", err.Error())
	}
	if len(chunks) == 0 {
		return true, l.Store.FailIngestionJob(ctx, job, "empty_chunks", "document extraction produced no chunks")
	}
	embeddingSetting, err := l.Store.FindProviderSetting(ctx, l.EmbeddingPurpose)
	if err != nil {
		return true, l.Store.FailIngestionJob(ctx, job, "embedding_provider_missing", err.Error())
	}
	embeddingStarted := time.Now()
	if l.RetrievalIndex != nil {
		if err := l.RetrievalIndex.Preflight(ctx, chunks); err != nil {
			return true, l.Store.FailIngestionJob(ctx, job, "embedding_failed", err.Error())
		}
	}
	l.recordMetric(ctx, "embedding_duration", time.Since(embeddingStarted), int64(len(chunks)), map[string]any{"document_id": doc.ID, "job_id": job.ID})

	if err := l.Store.CompleteIngestionJob(ctx, job, doc, domain.DocumentVersion{
		DocumentID:         doc.ID,
		MarkdownStorageKey: markdownKey,
		SchemaVersion:      pkg.SchemaVersion,
		MetadataJSON:       string(metadata),
		EmbeddingModel:     embeddingSetting.Model,
	}, chunks); err != nil {
		return true, err
	}
	l.recordMetric(ctx, "ingestion_duration", time.Since(started), int64(len(chunks)), map[string]any{"document_id": doc.ID, "job_id": job.ID, "chunk_count": len(chunks)})
	l.recordMetric(ctx, "ingestion_chunk_count", 0, int64(len(chunks)), map[string]any{"document_id": doc.ID, "job_id": job.ID})
	if l.RetrievalIndex != nil {
		return true, l.RetrievalIndex.Refresh(ctx)
	}
	return true, nil
}

func (l Lifecycle) recordMetric(ctx context.Context, name string, duration time.Duration, count int64, labels map[string]any) {
	if l.RecordMetric != nil {
		_ = l.RecordMetric(ctx, name, duration, count, labels)
	}
}

func formatID(id int64) string {
	return strconv.FormatInt(id, 10)
}
