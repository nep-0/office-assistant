package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	ingestionJobPending         = "pending"
	ingestionJobProcessing      = "processing"
	ingestionJobSucceeded       = "succeeded"
	ingestionJobFailed          = "failed"
	ingestionJobCancelRequested = "cancel_requested"
	ingestionJobCancelled       = "cancelled"
)

type extractionPackage struct {
	SchemaVersion string           `json:"schema_version"`
	Markdown      string           `json:"markdown"`
	Metadata      map[string]any   `json:"metadata"`
	Warnings      []string         `json:"warnings"`
	OCR           map[string]any   `json:"ocr"`
	SourceAnchors []map[string]any `json:"source_anchors"`
}

type extractedMarkdownResponse struct {
	DocumentID int64  `json:"document_id"`
	Markdown   string `json:"markdown"`
}

func (a *app) runIngestionWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				processed, err := a.processNextIngestionJob(ctx)
				if err != nil {
					break
				}
				if !processed {
					break
				}
			}
		}
	}
}

func (a *app) processNextIngestionJob(ctx context.Context) (bool, error) {
	job, doc, ok, err := a.store.claimNextIngestionJob(ctx)
	if err != nil || !ok {
		return ok, err
	}

	pkg, err := a.extractDocument(ctx, doc)
	if err != nil {
		return true, a.store.failIngestionJob(ctx, job, "document_extraction_failed", err.Error())
	}
	if pkg.Markdown == "" {
		return true, a.store.failIngestionJob(ctx, job, "empty_extraction", "document extraction returned no Markdown")
	}

	metadata, err := json.Marshal(map[string]any{
		"metadata":       pkg.Metadata,
		"warnings":       pkg.Warnings,
		"ocr":            pkg.OCR,
		"source_anchors": pkg.SourceAnchors,
	})
	if err != nil {
		return true, a.store.failIngestionJob(ctx, job, "metadata_encode_failed", err.Error())
	}

	markdownKey, err := a.writeExtractedMarkdown(doc, pkg.Markdown)
	if err != nil {
		return true, a.store.failIngestionJob(ctx, job, "artifact_store_failed", err.Error())
	}
	chunker := a.chunkingStrategy
	if chunker == nil {
		chunker = markdownChunkingStrategy{}
	}
	chunks, err := chunker.Chunk(pkg.Markdown, pkg.SourceAnchors)
	if err != nil {
		return true, a.store.failIngestionJob(ctx, job, "chunking_failed", err.Error())
	}
	if len(chunks) == 0 {
		return true, a.store.failIngestionJob(ctx, job, "empty_chunks", "document extraction produced no chunks")
	}
	embeddingSetting, err := a.store.findProviderSetting(ctx, providerPurposeEmbedding)
	if err != nil {
		return true, a.store.failIngestionJob(ctx, job, "embedding_provider_missing", err.Error())
	}
	if err := a.preflightVectorIndex(ctx, chunks); err != nil {
		return true, a.store.failIngestionJob(ctx, job, "embedding_failed", err.Error())
	}

	if err := a.store.completeIngestionJob(ctx, job, doc, documentVersion{
		DocumentID:         doc.ID,
		MarkdownStorageKey: markdownKey,
		SchemaVersion:      pkg.SchemaVersion,
		MetadataJSON:       string(metadata),
		EmbeddingModel:     embeddingSetting.Model,
	}, chunks); err != nil {
		return true, err
	}
	return true, a.rebuildVectorIndex(ctx)
}

func (a *app) extractDocument(ctx context.Context, doc documentRecord) (extractionPackage, error) {
	file, err := os.Open(filepath.Join(a.config.storageRoot, doc.StorageKey))
	if err != nil {
		return extractionPackage{}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", doc.OriginalFilename)
	if err != nil {
		return extractionPackage{}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return extractionPackage{}, err
	}
	if err := writer.Close(); err != nil {
		return extractionPackage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.documentURL+"/extract", &body)
	if err != nil {
		return extractionPackage{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return extractionPackage{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return extractionPackage{}, errUnexpectedStatus(res.Status)
	}

	var pkg extractionPackage
	if err := json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(&pkg); err != nil {
		return extractionPackage{}, err
	}
	return pkg, nil
}

func (a *app) writeExtractedMarkdown(doc documentRecord, markdown string) (string, error) {
	markdownKey := filepath.Join(filepath.Dir(doc.StorageKey), "extracted.md")
	fullPath := filepath.Join(a.config.storageRoot, markdownKey)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	return markdownKey, os.WriteFile(fullPath, []byte(markdown), 0o644)
}

func (a *app) cancelDocumentIngestion(w http.ResponseWriter, r *http.Request) {
	current, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	kb, err := a.store.findKnowledgeBaseByID(r.Context(), doc.KnowledgeBaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load knowledge base", nil)
		return
	}
	if !canModifyKnowledgeBase(current, kb) {
		writeError(w, http.StatusForbidden, "forbidden", "knowledge base write access required", nil)
		return
	}
	if err := a.store.cancelIngestionForDocument(r.Context(), doc.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not cancel ingestion", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancel_requested"})
}

func (a *app) getExtractedMarkdown(w http.ResponseWriter, r *http.Request) {
	_, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	version, err := a.store.findLatestDocumentVersion(r.Context(), doc.ID)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "extracted_markdown_not_found", "extracted Markdown is not available", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load extracted Markdown", nil)
		return
	}
	data, err := os.ReadFile(filepath.Join(a.config.storageRoot, version.MarkdownStorageKey))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifact_read_failed", "could not read extracted Markdown", nil)
		return
	}
	writeJSON(w, http.StatusOK, extractedMarkdownResponse{DocumentID: doc.ID, Markdown: string(data)})
}

func (a *app) authorizedDocument(w http.ResponseWriter, r *http.Request) (user, documentRecord, bool) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return user{}, documentRecord{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
		return user{}, documentRecord{}, false
	}
	doc, err := a.store.findDocumentByID(r.Context(), id)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
			return user{}, documentRecord{}, false
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load document", nil)
		return user{}, documentRecord{}, false
	}
	kb, err := a.store.findKnowledgeBaseByID(r.Context(), doc.KnowledgeBaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load knowledge base", nil)
		return user{}, documentRecord{}, false
	}
	if !canReadKnowledgeBase(current, kb) {
		writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
		return user{}, documentRecord{}, false
	}
	return current, doc, true
}

type unexpectedStatus string

func (err unexpectedStatus) Error() string {
	return "document service returned " + string(err)
}

func errUnexpectedStatus(status string) error {
	return unexpectedStatus(status)
}

func (a *app) preflightVectorIndex(ctx context.Context, chunks []documentChunk) error {
	if a.vectorIndex == nil {
		return nil
	}
	probe, err := newVectorIndex(a.embeddingFunc())
	if err != nil {
		return err
	}
	for i := range chunks {
		chunks[i].ID = int64(i + 1)
		chunks[i].DocumentID = 1
		chunks[i].DocumentVersionID = 1
	}
	return probe.rebuild(ctx, chunks)
}

func (a *app) rebuildVectorIndex(ctx context.Context) error {
	if a.vectorIndex == nil {
		return nil
	}
	chunks, err := a.store.listIndexedChunks(ctx)
	if err != nil {
		return err
	}
	return a.vectorIndex.rebuild(ctx, chunks)
}
