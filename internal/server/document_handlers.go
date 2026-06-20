package server

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"office-assistant/internal/app"
	"office-assistant/internal/auth"
)

func (a *api) listDocuments(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	docs, err := a.documents.List(r.Context(), claims.UserID, r.PathValue("id"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (a *api) uploadDocument(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
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

	doc, err := a.documents.Upload(r.Context(), app.UploadInput{
		KnowledgeBaseID: r.PathValue("id"),
		UserID:          claims.UserID,
		OriginalName:    header.Filename,
		ContentType:     contentType(header),
		File:            file,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, doc)
}

func contentType(header *multipart.FileHeader) string {
	if values := header.Header.Values("Content-Type"); len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	return "application/octet-stream"
}

func writeAppError(w http.ResponseWriter, err error) {
	var permission app.PermissionError
	if errors.As(err, &permission) {
		switch permission.Status {
		case "unauthorized":
			writeError(w, http.StatusUnauthorized, err)
		case "forbidden":
			writeError(w, http.StatusForbidden, err)
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	if app.IsConflict(err) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate document upload") {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}
