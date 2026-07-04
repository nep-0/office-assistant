package main

import (
	"context"
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
	a := newTestApp(t)
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

func TestAdminCanListMaskedProviderSettings(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", roleAdmin)

	res := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/provider-settings", "", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}

	var body providerSettingsResponse
	decodeRecorder(t, res, &body)
	if len(body.Settings) != 2 {
		t.Fatalf("expected two provider settings, got %+v", body.Settings)
	}
	for _, setting := range body.Settings {
		if setting.APIKeySet {
			t.Fatalf("expected default test setting without API key, got %+v", setting)
		}
		if strings.Contains(setting.APIKeyMask, "secret") {
			t.Fatalf("secret leaked in mask: %+v", setting)
		}
	}
}

func TestProviderSettingsRequireAdmin(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", roleMember)

	res := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/provider-settings", "", cookie)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestAdminCanSwitchProviderAndSecretIsMasked(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", roleAdmin)

	res := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/provider-settings/chat", `{
		"base_url":"https://api.example.test/v1",
		"model":"gpt-test",
		"api_key":"super-secret-key"
	}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}

	var body providerSettingResponse
	decodeRecorder(t, res, &body)
	if body.BaseURL != "https://api.example.test/v1" || body.Model != "gpt-test" {
		t.Fatalf("provider was not updated: %+v", body)
	}
	if !body.APIKeySet || body.APIKeyMask != "****-key" {
		t.Fatalf("expected masked secret, got %+v", body)
	}

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/provider-settings", "", cookie)
	if strings.Contains(list.Body.String(), "super-secret-key") {
		t.Fatalf("provider secret leaked in response: %s", list.Body.String())
	}

	ready := performJSON(t, a, http.MethodGet, "/api/ready", "")
	var readiness readinessResponse
	decodeRecorder(t, ready, &readiness)
	if readiness.Dependencies["chat_model"].URL != "https://api.example.test/v1" {
		t.Fatalf("readiness did not use active provider: %+v", readiness.Dependencies["chat_model"])
	}
}

func TestProviderSettingValidation(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", roleAdmin)

	res := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/provider-settings/chat", `{
		"base_url":"file:///tmp/model",
		"model":"gpt-test"
	}`, cookie)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	var body apiError
	decodeRecorder(t, res, &body)
	if body.Code != "provider_base_url_invalid" {
		t.Fatalf("expected provider_base_url_invalid, got %+v", body)
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := testConfig()
	if err := store.ensureProviderDefaults(context.Background(), cfg.defaultProviders); err != nil {
		t.Fatalf("seed provider defaults: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return &app{
		startedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		config:    cfg,
		store:     store,
	}
}

func testConfig() config {
	return config{
		documentURL:   "http://document:8081",
		ocrURL:        "http://ocr:8082",
		fakeProviders: true,
		defaultProviders: map[string]providerSetting{
			providerPurposeChat: {
				Purpose: providerPurposeChat,
				BaseURL: "http://backend:8080/fake-openai",
				Model:   "fake-chat",
			},
			providerPurposeEmbedding: {
				Purpose: providerPurposeEmbedding,
				BaseURL: "http://backend:8080/fake-openai",
				Model:   "fake-embedding",
			},
		},
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

func loginAs(t *testing.T, a *app, username, role string) *http.Cookie {
	t.Helper()
	createUserForTest(t, a, username, "password123", role)
	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"`+username+`","password":"password123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login as %s: %d %s", username, login.Code, login.Body.String())
	}
	return findCookie(t, login, sessionCookie)
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
