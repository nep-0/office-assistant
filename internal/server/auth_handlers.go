package server

import (
	"encoding/json"
	"net/http"

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
	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.accounts.BootstrapAdmin(r.Context(), request.Username, request.Password)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{Token: result.Token, User: publicUser(result.User)})
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.accounts.Login(r.Context(), request.Username, request.Password)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: result.Token, User: publicUser(result.User)})
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	user, err := a.accounts.Me(r.Context(), claims.UserID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (a *api) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.accounts.ListUsers(r.Context())
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
	user, err := a.accounts.CreateUser(r.Context(), request.Username, request.Password, request.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
}

func publicUser(user storage.User) publicUserResponse {
	return publicUserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
}
