package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"office-assistant/internal/auth"
	"office-assistant/internal/storage"
)

type knowledgeBaseRequest struct {
	Name string `json:"name"`
}

type addMemberRequest struct {
	UserID string `json:"user_id"`
}

func (a *api) createKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	var request knowledgeBaseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	kb, err := a.store.CreateKnowledgeBase(r.Context(), storage.KnowledgeBase{
		ID:        newID("kb"),
		Name:      strings.TrimSpace(request.Name),
		CreatedBy: claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, kb)
}

func (a *api) listKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("user no longer exists"))
		return
	}
	kbs, err := a.store.ListKnowledgeBases(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, kbs)
}

func (a *api) addKnowledgeBaseMember(w http.ResponseWriter, r *http.Request) {
	knowledgeBaseID := r.PathValue("id")
	var request addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.GetUserByID(r.Context(), request.UserID); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("user does not exist"))
		return
	}
	if err := a.store.AddKnowledgeBaseMember(r.Context(), knowledgeBaseID, request.UserID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"knowledge_base_id": knowledgeBaseID,
		"user_id":           request.UserID,
	})
}
