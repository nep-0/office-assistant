package app

import (
	"net/http"
	"strconv"
	"strings"

	authpkg "office-assistant/backend/auth"
	"office-assistant/backend/domain"
	knowledgepkg "office-assistant/backend/knowledge"
	"office-assistant/backend/utils"
)

const (
	visibilityPrivate = knowledgepkg.VisibilityPrivate
	visibilityPublic  = knowledgepkg.VisibilityPublic
)

type knowledgeBaseListResponse struct {
	KnowledgeBases []knowledgepkg.Response `json:"knowledge_bases"`
}

type createKnowledgeBaseRequest struct {
	Name string `json:"name"`
}

type updateKnowledgeBaseRequest struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility,omitempty"`
}

func (a *app) listKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	bases, err := a.store.ListKnowledgeBasesForUser(r.Context(), current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not load knowledge bases", nil)
		return
	}
	res := knowledgeBaseListResponse{KnowledgeBases: make([]knowledgepkg.Response, 0, len(bases))}
	for _, kb := range bases {
		res.KnowledgeBases = append(res.KnowledgeBases, a.toKnowledgeBaseResponse(kb, current))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) createKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	var req createKnowledgeBaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "knowledge_base_name_required", "knowledge base name is required", nil)
		return
	}

	kb, err := a.store.CreateKnowledgeBase(r.Context(), current.ID, name, visibilityPrivate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not create knowledge base", nil)
		return
	}
	writeJSON(w, http.StatusCreated, a.toKnowledgeBaseResponse(kb, current))
}

func (a *app) getKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.toKnowledgeBaseResponse(kb, current))
}

func (a *app) updateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	if !canModifyKnowledgeBase(current, kb) {
		writeError(w, http.StatusForbidden, "forbidden", "knowledge base write access required", nil)
		return
	}

	var req updateKnowledgeBaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "knowledge_base_name_required", "knowledge base name is required", nil)
		return
	}
	visibility := kb.Visibility
	if req.Visibility != "" {
		if current.Role != authpkg.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "admin role required to change knowledge base visibility", nil)
			return
		}
		if !validVisibility(req.Visibility) {
			writeError(w, http.StatusBadRequest, "knowledge_base_visibility_invalid", "knowledge base visibility must be private or public", nil)
			return
		}
		visibility = req.Visibility
	}

	updated, err := a.store.UpdateKnowledgeBase(r.Context(), kb.ID, name, visibility)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not update knowledge base", nil)
		return
	}
	writeJSON(w, http.StatusOK, a.toKnowledgeBaseResponse(updated, current))
}

func (a *app) deleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	current, kb, ok := a.authorizedKnowledgeBase(w, r)
	if !ok {
		return
	}
	if kb.Visibility == visibilityPublic && current.Role != authpkg.RoleAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required to delete public knowledge bases", nil)
		return
	}
	if !canModifyKnowledgeBase(current, kb) {
		writeError(w, http.StatusForbidden, "forbidden", "knowledge base write access required", nil)
		return
	}
	if err := a.store.DeleteKnowledgeBase(r.Context(), kb.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not delete knowledge base", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) authorizedKnowledgeBase(w http.ResponseWriter, r *http.Request) (domain.User, domain.KnowledgeBase, bool) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return domain.User{}, domain.KnowledgeBase{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "knowledge_base_not_found", "knowledge base not found", nil)
		return domain.User{}, domain.KnowledgeBase{}, false
	}
	kb, err := a.store.FindKnowledgeBaseByID(r.Context(), id)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "knowledge_base_not_found", "knowledge base not found", nil)
			return domain.User{}, domain.KnowledgeBase{}, false
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load knowledge base", nil)
		return domain.User{}, domain.KnowledgeBase{}, false
	}
	if !canReadKnowledgeBase(current, kb) {
		writeError(w, http.StatusNotFound, "knowledge_base_not_found", "knowledge base not found", nil)
		return domain.User{}, domain.KnowledgeBase{}, false
	}
	return current, kb, true
}

func (a *app) toKnowledgeBaseResponse(kb domain.KnowledgeBase, current domain.User) knowledgepkg.Response {
	return knowledgepkg.ToResponse(kb, current)
}

func canReadKnowledgeBase(current domain.User, kb domain.KnowledgeBase) bool {
	return knowledgepkg.CanRead(current, kb)
}

func canModifyKnowledgeBase(current domain.User, kb domain.KnowledgeBase) bool {
	return knowledgepkg.CanModify(current, kb)
}

func validVisibility(visibility string) bool {
	return knowledgepkg.ValidVisibility(visibility)
}
