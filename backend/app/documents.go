package app

import (
	"net/http"
	"path/filepath"
	"strings"

	docpkg "office-assistant/backend/documents"
	"office-assistant/backend/domain"
	storepkg "office-assistant/backend/store"
)

const (
	documentStatusPending    = docpkg.StatusPending
	documentStatusProcessing = docpkg.StatusProcessing
	documentStatusReady      = docpkg.StatusReady
	documentStatusFailed     = docpkg.StatusFailed
	documentStatusCancelled  = docpkg.StatusCancelled
	maxUploadBytes           = 50 << 20
)

type documentListResponse struct {
	Documents []docpkg.Response `json:"documents"`
}

func (a *app) listDocuments(w http.ResponseWriter, r *http.Request) {
	_, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	docs, err := a.store.ListDocumentsForKnowledgeBase(r.Context(), kb.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load documents", nil)
		return
	}
	res := documentListResponse{Documents: make([]docpkg.Response, 0, len(docs))}
	for _, doc := range docs {
		res.Documents = append(res.Documents, toDocumentResponse(doc))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) searchDocuments(w http.ResponseWriter, r *http.Request) {
	_, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := storepkg.DocumentSearchFilter{
		Query:       ftsQuery(query),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		ContentType: strings.TrimSpace(r.URL.Query().Get("type")),
		DateFrom:    strings.TrimSpace(r.URL.Query().Get("from")),
		DateTo:      strings.TrimSpace(r.URL.Query().Get("to")),
	}
	docs, err := a.store.SearchDocuments(r.Context(), kb.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not search documents", nil)
		return
	}
	res := documentListResponse{Documents: make([]docpkg.Response, 0, len(docs))}
	for _, doc := range docs {
		res.Documents = append(res.Documents, toDocumentResponse(doc))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) uploadDocument(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "upload_invalid", "upload must include one supported file", nil)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload_file_required", "file field is required", nil)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	doc, err := a.documentLifecycle().Upload(r.Context(), docpkg.UploadInput{
		Current:          current,
		KnowledgeBase:    kb,
		File:             file,
		OriginalFilename: header.Filename,
		ContentType:      strings.TrimSpace(contentType),
		ConfirmDuplicate: r.URL.Query().Get("confirm_duplicate") == "true",
	})
	if err != nil {
		a.writeDocumentLifecycleUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDocumentResponse(doc))
}

func (a *app) deleteDocument(w http.ResponseWriter, r *http.Request) {
	current, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	if err := a.documentLifecycle().Delete(r.Context(), current, doc); err != nil {
		a.writeDocumentLifecycleMutationError(w, err, "could not delete document")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (a *app) reprocessDocument(w http.ResponseWriter, r *http.Request) {
	current, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	if err := a.documentLifecycle().Reprocess(r.Context(), current, doc); err != nil {
		a.writeDocumentLifecycleMutationError(w, err, "could not schedule reprocessing")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

func (a *app) downloadDocument(w http.ResponseWriter, r *http.Request) {
	_, doc, ok := a.authorizedDocument(w, r)
	if !ok {
		return
	}
	path := filepath.Join(a.config.storageRoot, doc.StorageKey)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(doc.OriginalFilename, `"`, "")+`"`)
	w.Header().Set("Content-Type", doc.ContentType)
	http.ServeFile(w, r, path)
}

func cleanFilename(filename string) string {
	return docpkg.CleanFilename(filename)
}

func supportedOfficeInput(filename string) bool {
	return docpkg.SupportedOfficeInput(filename)
}

func ftsQuery(query string) string {
	return docpkg.FTSQuery(query)
}

func toDocumentResponse(doc domain.DocumentRecord) docpkg.Response {
	return docpkg.ToResponse(doc)
}

func (a *app) writeDocumentLifecycleUploadError(w http.ResponseWriter, err error) {
	if duplicate, ok := err.(docpkg.DuplicateError); ok {
		writeError(w, http.StatusConflict, "duplicate_document", "matching content already exists in this knowledge base", map[string]any{
			"duplicate": toDocumentResponse(duplicate.Duplicate),
		})
		return
	}
	if lifecycleErr, ok := err.(docpkg.LifecycleError); ok {
		status := http.StatusBadRequest
		if lifecycleErr.Code == "forbidden" {
			status = http.StatusForbidden
		}
		writeError(w, status, lifecycleErr.Code, lifecycleErr.Message, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "upload_store_error", "could not store upload", nil)
}

func (a *app) writeDocumentLifecycleMutationError(w http.ResponseWriter, err error, message string) {
	if lifecycleErr, ok := err.(docpkg.LifecycleError); ok {
		status := http.StatusInternalServerError
		if lifecycleErr.Code == "forbidden" {
			status = http.StatusForbidden
		}
		if lifecycleErr.Code == "document_not_found" {
			status = http.StatusNotFound
		}
		writeError(w, status, lifecycleErr.Code, lifecycleErr.Message, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "store_error", message, nil)
}
