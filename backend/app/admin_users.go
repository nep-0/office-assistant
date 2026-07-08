package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	authpkg "office-assistant/backend/auth"
	"office-assistant/backend/utils"
)

type adminUsersResponse struct {
	Users []authUserResponse `json:"users"`
}

type adminCreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type adminUpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	Role     *string `json:"role,omitempty"`
}

func (a *app) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not list users", nil)
		return
	}
	res := adminUsersResponse{Users: make([]authUserResponse, 0, len(users))}
	for _, user := range users {
		res.Users = append(res.Users, toAuthUser(user))
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *app) createUser(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var req adminCreateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		req.Role = authpkg.RoleMember
	}
	created, err := a.createUserWithPassword(r, req.Username, req.Password, req.Role)
	if err != nil {
		writeAdminUserError(w, err, "could not create user")
		return
	}
	_ = a.store.AppendActivity(r.Context(), current.ID, "user_created", "user", strconv.FormatInt(created.ID, 10), map[string]any{"role": created.Role})
	writeJSON(w, http.StatusCreated, authResponse{User: toAuthUser(created)})
}

func (a *app) updateUser(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseUserID(w, r)
	if !ok {
		return
	}
	var req adminUpdateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}
	user, err := a.store.FindUserByID(r.Context(), id)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "user_not_found", "user not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load user", nil)
		return
	}
	if req.Username != nil {
		user.Username = normalizeUsername(*req.Username)
		if user.Username == "" {
			writeError(w, http.StatusBadRequest, "username_required", "username is required", nil)
			return
		}
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters", nil)
			return
		}
		hash, err := authpkg.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password_hash_error", "could not hash password", nil)
			return
		}
		user.PasswordHash = hash
	}
	if req.Role != nil {
		role := strings.TrimSpace(*req.Role)
		if !validRole(role) {
			writeError(w, http.StatusBadRequest, "role_invalid", "role must be admin or member", nil)
			return
		}
		if user.ID == current.ID && user.Role == authpkg.RoleAdmin && role != authpkg.RoleAdmin {
			writeError(w, http.StatusBadRequest, "cannot_demote_self", "admins cannot demote themselves", nil)
			return
		}
		if user.Role == authpkg.RoleAdmin && role != authpkg.RoleAdmin {
			if ok := a.canRemoveAdmin(w, r); !ok {
				return
			}
		}
		user.Role = role
	}
	updated, err := a.store.UpdateUser(r.Context(), user)
	if err != nil {
		writeAdminUserError(w, err, "could not update user")
		return
	}
	_ = a.store.AppendActivity(r.Context(), current.ID, "user_updated", "user", strconv.FormatInt(updated.ID, 10), map[string]any{"role": updated.Role})
	writeJSON(w, http.StatusOK, authResponse{User: toAuthUser(updated)})
}

func (a *app) deleteUser(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if id == current.ID {
		writeError(w, http.StatusBadRequest, "cannot_delete_self", "admins cannot delete themselves", nil)
		return
	}
	user, err := a.store.FindUserByID(r.Context(), id)
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "user_not_found", "user not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load user", nil)
		return
	}
	if user.Role == authpkg.RoleAdmin {
		if ok := a.canRemoveAdmin(w, r); !ok {
			return
		}
	}
	owned, err := a.store.CountKnowledgeBasesOwnedByUser(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not inspect user ownership", nil)
		return
	}
	if owned > 0 {
		writeError(w, http.StatusConflict, "user_owns_knowledge_bases", "user owns knowledge bases and cannot be deleted", nil)
		return
	}
	if err := a.store.DeleteUser(r.Context(), user.ID); err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusNotFound, "user_not_found", "user not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not delete user", nil)
		return
	}
	_ = a.store.AppendActivity(r.Context(), current.ID, "user_deleted", "user", strconv.FormatInt(user.ID, 10), map[string]any{"role": user.Role})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func parseUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "user id must be a positive integer", nil)
		return 0, false
	}
	return id, true
}

func (a *app) canRemoveAdmin(w http.ResponseWriter, r *http.Request) bool {
	count, err := a.store.CountAdmins(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not inspect admin users", nil)
		return false
	}
	if count <= 1 {
		writeError(w, http.StatusBadRequest, "last_admin_required", "at least one admin must remain", nil)
		return false
	}
	return true
}

func writeAdminUserError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, errInvalidRole):
		writeError(w, http.StatusBadRequest, "role_invalid", "role must be admin or member", nil)
	case err.Error() == "username_required":
		writeError(w, http.StatusBadRequest, "username_required", "username is required", nil)
	case err.Error() == "password_too_short":
		writeError(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters", nil)
	case isUniqueConstraint(err):
		writeError(w, http.StatusConflict, "username_taken", "username is already taken", nil)
	default:
		writeError(w, http.StatusInternalServerError, "store_error", fallback, nil)
	}
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

var errInvalidRole = errors.New("role_invalid")

func validRole(role string) bool {
	return role == authpkg.RoleAdmin || role == authpkg.RoleMember
}
