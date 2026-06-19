package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"office-assistant/internal/auth"
	"office-assistant/internal/storage"
)

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string             `json:"token"`
	User  publicUserResponse `json:"user"`
}

type publicUserResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (a *api) bootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, errors.New("bootstrap is only available before users exist"))
		return
	}

	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := a.createUserWithPassword(r.Context(), request.Username, request.Password, "admin")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	token, err := a.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{Token: token, User: publicUser(user)})
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := a.store.GetUserByUsername(r.Context(), strings.TrimSpace(request.Username))
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid username or password"))
		return
	}
	if err := auth.CheckPassword(user.PasswordHash, request.Password); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	token, err := a.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, User: publicUser(user)})
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("user no longer exists"))
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (a *api) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response := make([]publicUserResponse, 0, len(users))
	for _, user := range users {
		response = append(response, publicUser(user))
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *api) createUser(w http.ResponseWriter, r *http.Request) {
	var request createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := a.createUserWithPassword(r.Context(), request.Username, request.Password, request.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
}

func (a *api) createUserWithPassword(ctx context.Context, username, password, role string) (storage.User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return storage.User{}, err
	}
	return a.store.CreateUser(ctx, storage.User{
		ID:           newID("usr"),
		Username:     strings.TrimSpace(username),
		PasswordHash: hash,
		Role:         strings.TrimSpace(role),
	})
}

func publicUser(user storage.User) publicUserResponse {
	return publicUserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
}
