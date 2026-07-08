package app

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	authpkg "office-assistant/backend/auth"
	"office-assistant/backend/domain"
	"office-assistant/backend/utils"
)

type setupStatusResponse struct {
	NeedsSetup bool `json:"needs_setup"`
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authUserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type authResponse struct {
	User authUserResponse `json:"user"`
}

func (a *app) setupStatus(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not inspect setup state", nil)
		return
	}
	writeJSON(w, http.StatusOK, setupStatusResponse{NeedsSetup: count == 0})
}

func (a *app) createFirstAdmin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}

	count, err := a.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not inspect setup state", nil)
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup_completed", "first admin has already been created", nil)
		return
	}

	created, err := a.createUserWithPassword(r, req.Username, req.Password, authpkg.RoleAdmin)
	if err != nil {
		writeAuthCreateError(w, err)
		return
	}
	_ = a.store.AppendActivity(r.Context(), created.ID, "user_created", "user", strconv.FormatInt(created.ID, 10), map[string]any{"role": created.Role, "first_admin": true})

	if err := a.issueSession(w, r, created); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session", nil)
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{User: toAuthUser(created)})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
		return
	}

	found, err := a.store.FindUserByUsername(r.Context(), normalizeUsername(req.Username))
	if err != nil {
		if utils.NotFound(err) {
			_ = a.store.AppendActivity(r.Context(), 0, "login_failed", "user", normalizeUsername(req.Username), map[string]any{"reason": "unknown_user"})
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "username or password is incorrect", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load user", nil)
		return
	}
	if !authpkg.CheckPassword(found.PasswordHash, req.Password) {
		_ = a.store.AppendActivity(r.Context(), found.ID, "login_failed", "user", strconv.FormatInt(found.ID, 10), map[string]any{"reason": "bad_password"})
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "username or password is incorrect", nil)
		return
	}
	if err := a.issueSession(w, r, found); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session", nil)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: toAuthUser(found)})
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(authpkg.SessionCookie); err == nil {
		_ = a.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, expiredSessionCookie())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: toAuthUser(current)})
}

func (a *app) adminStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "role": authpkg.RoleAdmin})
}

func (a *app) requireAdmin(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	current, ok := a.currentUser(w, r)
	if !ok {
		return domain.User{}, false
	}
	if current.Role != authpkg.RoleAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required", nil)
		return domain.User{}, false
	}
	return current, true
}

func (a *app) createUserWithPassword(r *http.Request, username, password, role string) (domain.User, error) {
	username = normalizeUsername(username)
	if username == "" {
		return domain.User{}, errors.New("username_required")
	}
	if len(password) < 8 {
		return domain.User{}, errors.New("password_too_short")
	}
	if !validRole(role) {
		return domain.User{}, errInvalidRole
	}
	hash, err := authpkg.HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	return a.store.CreateUser(r.Context(), username, hash, role)
}

func writeAuthCreateError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "username_required":
		writeError(w, http.StatusBadRequest, "username_required", "username is required", nil)
	case "password_too_short":
		writeError(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters", nil)
	default:
		writeError(w, http.StatusInternalServerError, "store_error", "could not create user", nil)
	}
}

func (a *app) issueSession(w http.ResponseWriter, r *http.Request, u domain.User) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(authpkg.SessionTTL)
	if err := a.store.CreateSession(r.Context(), token, u.ID, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, activeSessionCookie(token, expiresAt))
	return nil
}

func (a *app) currentUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	cookie, err := r.Cookie(authpkg.SessionCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "login required", nil)
		return domain.User{}, false
	}
	current, err := a.store.FindUserBySession(r.Context(), cookie.Value, time.Now().UTC())
	if err != nil {
		if utils.NotFound(err) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "login required", nil)
			return domain.User{}, false
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load session", nil)
		return domain.User{}, false
	}
	return current, true
}

func activeSessionCookie(token string, expiresAt time.Time) *http.Cookie {
	return authpkg.ActiveSessionCookie(token, expiresAt)
}

func expiredSessionCookie() *http.Cookie {
	return authpkg.ExpiredSessionCookie()
}

func randomToken() (string, error) {
	return authpkg.RandomToken()
}

func normalizeUsername(username string) string {
	return authpkg.NormalizeUsername(username)
}

func toAuthUser(u domain.User) authUserResponse {
	return authUserResponse{
		ID:       u.ID,
		Username: u.Username,
		Role:     u.Role,
	}
}
