package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	roleAdmin     = "admin"
	roleMember    = "member"
	sessionCookie = "oa_session"
	sessionTTL    = 24 * time.Hour
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
	count, err := a.store.countUsers(r.Context())
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

	count, err := a.store.countUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not inspect setup state", nil)
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup_completed", "first admin has already been created", nil)
		return
	}

	created, err := a.createUserWithPassword(r, req.Username, req.Password, roleAdmin)
	if err != nil {
		writeAuthCreateError(w, err)
		return
	}

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

	found, err := a.store.findUserByUsername(r.Context(), normalizeUsername(req.Username))
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "username or password is incorrect", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load user", nil)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(req.Password)) != nil {
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
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = a.store.deleteSession(r.Context(), cookie.Value)
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
	current, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	if current.Role != roleAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "role": roleAdmin})
}

func (a *app) createUserWithPassword(r *http.Request, username, password, role string) (user, error) {
	username = normalizeUsername(username)
	if username == "" {
		return user{}, errors.New("username_required")
	}
	if len(password) < 8 {
		return user{}, errors.New("password_too_short")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return user{}, err
	}
	return a.store.createUser(r.Context(), username, string(hash), role)
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

func (a *app) issueSession(w http.ResponseWriter, r *http.Request, u user) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	if err := a.store.createSession(r.Context(), token, u.ID, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, activeSessionCookie(token, expiresAt))
	return nil
}

func (a *app) currentUser(w http.ResponseWriter, r *http.Request) (user, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "login required", nil)
		return user{}, false
	}
	current, err := a.store.findUserBySession(r.Context(), cookie.Value, time.Now().UTC())
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "login required", nil)
			return user{}, false
		}
		writeError(w, http.StatusInternalServerError, "store_error", "could not load session", nil)
		return user{}, false
	}
	return current, true
}

func activeSessionCookie(token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func randomToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func toAuthUser(u user) authUserResponse {
	return authUserResponse{
		ID:       u.ID,
		Username: u.Username,
		Role:     u.Role,
	}
}
