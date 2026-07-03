package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	a := &app{startedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	res := httptest.NewRecorder()

	a.health(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" || body.Service != "backend" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestReadyIncludesFakeProviders(t *testing.T) {
	a := &app{
		startedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		config: config{
			documentURL:   "http://document:8081",
			ocrURL:        "http://ocr:8082",
			fakeProviders: true,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	res := httptest.NewRecorder()

	a.ready(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var body readinessResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("expected ready status, got %q", body.Status)
	}
	if body.Dependencies["chat_model"].Mode != "fake" {
		t.Fatalf("expected fake chat provider, got %+v", body.Dependencies["chat_model"])
	}
	if body.Dependencies["embedding_model"].Mode != "fake" {
		t.Fatalf("expected fake embedding provider, got %+v", body.Dependencies["embedding_model"])
	}
}

func TestFirstRunSetupCreatesAdminAndSession(t *testing.T) {
	a := newTestApp(t)

	status := performJSON(t, a, http.MethodGet, "/api/setup/status", "")
	if status.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status.Code)
	}
	var setup setupStatusResponse
	decodeRecorder(t, status, &setup)
	if !setup.NeedsSetup {
		t.Fatal("expected setup to be required before users exist")
	}

	created := performJSON(t, a, http.MethodPost, "/api/setup", `{"username":"Admin","password":"password123"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, created.Code, created.Body.String())
	}
	cookie := findCookie(t, created, sessionCookie)
	if !cookie.HttpOnly {
		t.Fatal("expected session cookie to be HTTP-only")
	}

	me := performJSONWithCookie(t, a, http.MethodGet, "/api/auth/me", "", cookie)
	if me.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, me.Code)
	}
	var auth authResponse
	decodeRecorder(t, me, &auth)
	if auth.User.Username != "admin" || auth.User.Role != roleAdmin {
		t.Fatalf("unexpected user: %+v", auth.User)
	}

	status = performJSON(t, a, http.MethodGet, "/api/setup/status", "")
	decodeRecorder(t, status, &setup)
	if setup.NeedsSetup {
		t.Fatal("expected setup to be complete")
	}
}

func TestLoginLogoutAndProtectedAdminRoute(t *testing.T) {
	a := newTestApp(t)
	createUserForTest(t, a, "admin", "password123", roleAdmin)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"password123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, login.Code, login.Body.String())
	}
	cookie := findCookie(t, login, sessionCookie)

	admin := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/status", "", cookie)
	if admin.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, admin.Code, admin.Body.String())
	}

	logout := performJSONWithCookie(t, a, http.MethodPost, "/api/auth/logout", `{}`, cookie)
	if logout.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, logout.Code)
	}

	me := performJSONWithCookie(t, a, http.MethodGet, "/api/auth/me", "", cookie)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d after logout, got %d", http.StatusUnauthorized, me.Code)
	}
}

func TestMemberCannotUseAdminRoute(t *testing.T) {
	a := newTestApp(t)
	createUserForTest(t, a, "member", "password123", roleMember)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"member","password":"password123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, login.Code, login.Body.String())
	}
	cookie := findCookie(t, login, sessionCookie)

	admin := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/status", "", cookie)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, admin.Code)
	}

	var body apiError
	decodeRecorder(t, admin, &body)
	if body.Code != "forbidden" {
		t.Fatalf("expected stable forbidden code, got %+v", body)
	}
}

func TestInvalidLoginReturnsStructuredError(t *testing.T) {
	a := newTestApp(t)
	createUserForTest(t, a, "admin", "password123", roleAdmin)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`)
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, login.Code)
	}

	var body apiError
	decodeRecorder(t, login, &body)
	if body.Code != "invalid_credentials" || body.Message == "" {
		t.Fatalf("expected structured error, got %+v", body)
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return &app{
		startedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		config: config{
			documentURL:   "http://document:8081",
			ocrURL:        "http://ocr:8082",
			fakeProviders: true,
		},
		store: store,
	}
}

func createUserForTest(t *testing.T, a *app, username, password, role string) user {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	created, err := a.createUserWithPassword(req, username, password, role)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return created
}

func performJSON(t *testing.T, a *app, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return performJSONWithCookie(t, a, method, path, body, nil)
}

func performJSONWithCookie(t *testing.T, a *app, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()

	mux := http.NewServeMux()
	a.routes(mux)
	mux.ServeHTTP(res, req)
	return res
}

func decodeRecorder(t *testing.T, res *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func findCookie(t *testing.T, res *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range res.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q not found", name)
	return nil
}
