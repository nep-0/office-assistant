package app

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"office-assistant/backend/domain"
	"office-assistant/backend/utils"
)

const (
	ingestionJobPending         = "pending"
	ingestionJobProcessing      = "processing"
	ingestionJobSucceeded       = "succeeded"
	ingestionJobFailed          = "failed"
	ingestionJobCancelRequested = "cancel_requested"
	ingestionJobCancelled       = "cancelled"
)

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
	processed, err := a.documentLifecycle().ProcessNextIngestionJob(ctx)
	if processed {
		log.Printf("event=ingestion_job_processed")
	}
	return processed, err
}

func (a *app) cancelDocumentIngestion(w http.ResponseWriter, r *http.Request) {
	current, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	if err := a.documentLifecycle().CancelIngestion(r.Context(), current, doc); err != nil {
		a.writeDocumentLifecycleMutationError(w, err, "could not cancel ingestion")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancel_requested"})
}

func (a *app) getExtractedMarkdown(w http.ResponseWriter, r *http.Request) {
	current, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	markdown, err := a.documentLifecycle().ExtractedMarkdown(r.Context(), current, doc)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "extracted_markdown_not_found", "extracted Markdown is not available", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load extracted Markdown", nil)
		return
	}
	writeJSON(w, http.StatusOK, extractedMarkdownResponse{DocumentID: doc.ID, Markdown: markdown})
}

func (a *app) authorizedDocument(w http.ResponseWriter, r *http.Request) (domain.User, domain.DocumentRecord, bool) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return domain.User{}, domain.DocumentRecord{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
		return domain.User{}, domain.DocumentRecord{}, false
	}
	doc, err := a.store.FindDocumentByID(r.Context(), id)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
			return domain.User{}, domain.DocumentRecord{}, false
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load document", nil)
		return domain.User{}, domain.DocumentRecord{}, false
	}
	kb, err := a.store.FindKnowledgeBaseByID(r.Context(), doc.KnowledgeBaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load knowledge base", nil)
		return domain.User{}, domain.DocumentRecord{}, false
	}
	if !canReadKnowledgeBase(current, kb) {
		writeError(w, http.StatusNotFound, "document_not_found", "document not found", nil)
		return domain.User{}, domain.DocumentRecord{}, false
	}
	return current, doc, true
}

func (a *app) preflightVectorIndex(ctx context.Context, chunks []domain.DocumentChunk) error {
	if a.vectorIndex == nil {
		return nil
	}
	return a.vectorIndex.Preflight(ctx, chunks)
}

func (a *app) rebuildVectorIndex(ctx context.Context) error {
	if a.vectorIndex == nil {
		return nil
	}
	chunks, err := a.store.ListIndexedChunks(ctx)
	if err != nil {
		return err
	}
	return a.vectorIndex.Rebuild(ctx, chunks)
}
