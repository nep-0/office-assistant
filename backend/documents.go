package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	documentStatusPending    = "pending"
	documentStatusProcessing = "processing"
	documentStatusReady      = "ready"
	documentStatusFailed     = "failed"
	documentStatusCancelled  = "cancelled"
	maxUploadBytes           = 50 << 20
)

var supportedOfficeExtensions = map[string]bool{
	".pdf":  true,
	".docx": true,
	".xlsx": true,
	".pptx": true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".tif":  true,
	".tiff": true,
	".webp": true,
}

type documentResponse struct {
	ID               int64  `json:"id"`
	KnowledgeBaseID  int64  `json:"knowledge_base_id"`
	OriginalFilename string `json:"original_filename"`
	DisplayName      string `json:"display_name"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	SHA256           string `json:"sha256"`
	Status           string `json:"status"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type documentListResponse struct {
	Documents []documentResponse `json:"documents"`
}

func (a *app) listDocuments(w http.ResponseWriter, r *http.Request) {
	_, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	docs, err := a.store.listDocumentsForKnowledgeBase(r.Context(), kb.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load documents", nil)
		return
	}
	res := documentListResponse{Documents: make([]documentResponse, 0, len(docs))}
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
	filter := documentSearchFilter{
		Query:       ftsQuery(query),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		ContentType: strings.TrimSpace(r.URL.Query().Get("type")),
		DateFrom:    strings.TrimSpace(r.URL.Query().Get("from")),
		DateTo:      strings.TrimSpace(r.URL.Query().Get("to")),
	}
	docs, err := a.store.searchDocuments(r.Context(), kb.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not search documents", nil)
		return
	}
	res := documentListResponse{Documents: make([]documentResponse, 0, len(docs))}
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
	if !canModifyKnowledgeBase(current, kb) {
		writeError(w, http.StatusForbidden, "forbidden", "knowledge base write access required", nil)
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

	originalFilename := cleanFilename(header.Filename)
	if !supportedOfficeInput(originalFilename) {
		writeError(w, http.StatusBadRequest, "unsupported_office_input", "file type is not supported", nil)
		return
	}

	tempPath, hash, size, err := a.writeUploadTemp(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_store_error", "could not store upload", nil)
		return
	}
	defer os.Remove(tempPath)

	duplicate, err := a.store.findDocumentDuplicateInKnowledgeBase(r.Context(), kb.ID, hash)
	if err != nil && !notFound(err) {
		writeError(w, http.StatusInternalServerError, "store_error", "could not check duplicate document", nil)
		return
	}
	if err == nil && r.URL.Query().Get("confirm_duplicate") != "true" {
		writeError(w, http.StatusConflict, "duplicate_document", "matching content already exists in this knowledge base", map[string]any{
			"duplicate": toDocumentResponse(duplicate),
		})
		return
	}

	storageKey, finalPath, err := a.newDocumentStoragePath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_store_error", "could not prepare document storage", nil)
		return
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		writeError(w, http.StatusInternalServerError, "upload_store_error", "could not store upload", nil)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	doc, err := a.store.createDocument(r.Context(), documentRecord{
		KnowledgeBaseID:  kb.ID,
		OwnerID:          current.ID,
		OriginalFilename: originalFilename,
		DisplayName:      originalFilename,
		ContentType:      contentType,
		SizeBytes:        size,
		SHA256:           hash,
		StorageKey:       storageKey,
		Status:           documentStatusPending,
	})
	if err != nil {
		_ = os.Remove(finalPath)
		writeError(w, http.StatusInternalServerError, "store_error", "could not create document record", nil)
		return
	}
	if _, err := a.store.createIngestionJob(r.Context(), doc.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not create ingestion job", nil)
		return
	}
	_ = a.store.appendActivity(r.Context(), current.ID, "document_uploaded", "document", strconv.FormatInt(doc.ID, 10), map[string]any{"knowledge_base_id": kb.ID, "filename": doc.DisplayName})
	writeJSON(w, http.StatusCreated, toDocumentResponse(doc))
}

func (a *app) deleteDocument(w http.ResponseWriter, r *http.Request) {
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
	if err := a.store.deleteDocument(r.Context(), doc.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not delete document", nil)
		return
	}
	if err := a.rebuildVectorIndex(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "index_error", "could not rebuild vector index", nil)
		return
	}
	_ = a.store.appendActivity(r.Context(), current.ID, "document_deleted", "document", strconv.FormatInt(doc.ID, 10), map[string]any{"knowledge_base_id": kb.ID, "filename": doc.DisplayName})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (a *app) reprocessDocument(w http.ResponseWriter, r *http.Request) {
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
	if _, err := a.store.reprocessDocument(r.Context(), doc.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not schedule reprocessing", nil)
		return
	}
	_ = a.store.appendActivity(r.Context(), current.ID, "document_reprocess_requested", "document", strconv.FormatInt(doc.ID, 10), map[string]any{"knowledge_base_id": kb.ID, "filename": doc.DisplayName})
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

func (a *app) writeUploadTemp(file io.Reader) (string, string, int64, error) {
	if err := os.MkdirAll(filepath.Join(a.config.storageRoot, "tmp"), 0o755); err != nil {
		return "", "", 0, err
	}
	temp, err := os.CreateTemp(filepath.Join(a.config.storageRoot, "tmp"), "upload-*")
	if err != nil {
		return "", "", 0, err
	}
	defer temp.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hasher), file)
	if err != nil {
		_ = os.Remove(temp.Name())
		return "", "", 0, err
	}
	if written == 0 {
		_ = os.Remove(temp.Name())
		return "", "", 0, errors.New("empty upload")
	}
	return temp.Name(), hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func (a *app) newDocumentStoragePath() (string, string, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	storageKey := filepath.Join("documents", token, "original")
	fullPath := filepath.Join(a.config.storageRoot, storageKey)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", "", err
	}
	return storageKey, fullPath, nil
}

func cleanFilename(filename string) string {
	cleaned := filepath.Base(strings.TrimSpace(filename))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return "upload"
	}
	return cleaned
}

func supportedOfficeInput(filename string) bool {
	return supportedOfficeExtensions[strings.ToLower(filepath.Ext(filename))]
}

func ftsQuery(query string) string {
	terms := strings.Fields(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		quoted = append(quoted, `"`+term+`"`)
	}
	return strings.Join(quoted, " ")
}

func toDocumentResponse(doc documentRecord) documentResponse {
	return documentResponse{
		ID:               doc.ID,
		KnowledgeBaseID:  doc.KnowledgeBaseID,
		OriginalFilename: doc.OriginalFilename,
		DisplayName:      doc.DisplayName,
		ContentType:      doc.ContentType,
		SizeBytes:        doc.SizeBytes,
		SHA256:           doc.SHA256,
		Status:           doc.Status,
		ErrorCode:        doc.ErrorCode,
		ErrorMessage:     doc.ErrorMessage,
		CreatedAt:        doc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        doc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
