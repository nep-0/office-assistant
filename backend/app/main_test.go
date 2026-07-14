package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	authpkg "office-assistant/backend/auth"
	chatpkg "office-assistant/backend/chat"
	docpkg "office-assistant/backend/documents"
	"office-assistant/backend/domain"
	"office-assistant/backend/httpapi"
	ingestionpkg "office-assistant/backend/ingestion"
	knowledgepkg "office-assistant/backend/knowledge"
	"office-assistant/backend/providers"
	"office-assistant/backend/search"
	"office-assistant/backend/utils"

	"github.com/nep-0/harness/agent"
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

func TestReadyProbesConfiguredProviders(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected provider path %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
	}))
	t.Cleanup(provider.Close)

	a := newTestApp(t)
	a.config.fakeProviders = false
	if _, err := a.store.UpdateProviderSetting(context.Background(), domain.ProviderSetting{
		Purpose: providerPurposeChat,
		BaseURL: provider.URL + "/v1",
		Model:   "local-chat",
	}); err != nil {
		t.Fatalf("update chat provider: %v", err)
	}
	if _, err := a.store.UpdateProviderSetting(context.Background(), domain.ProviderSetting{
		Purpose: providerPurposeEmbedding,
		BaseURL: provider.URL + "/v1",
		Model:   "local-embedding",
	}); err != nil {
		t.Fatalf("update embedding provider: %v", err)
	}

	res := performJSON(t, a, http.MethodGet, "/api/ready", "")
	var body readinessResponse
	decodeRecorder(t, res, &body)
	if body.Status != "ready" {
		t.Fatalf("expected ready status, got %+v", body)
	}
	if body.Dependencies["chat_model"].Mode != "openai-compatible" {
		t.Fatalf("expected openai-compatible mode, got %+v", body.Dependencies["chat_model"])
	}
}

func TestReadyDegradesWhenProviderUnavailable(t *testing.T) {
	a := newTestApp(t)
	a.config.fakeProviders = false
	if _, err := a.store.UpdateProviderSetting(context.Background(), domain.ProviderSetting{
		Purpose: providerPurposeChat,
		BaseURL: "http://127.0.0.1:1/v1",
		Model:   "local-chat",
	}); err != nil {
		t.Fatalf("update chat provider: %v", err)
	}

	res := performJSON(t, a, http.MethodGet, "/api/ready", "")
	var body readinessResponse
	decodeRecorder(t, res, &body)
	if body.Status != "degraded" {
		t.Fatalf("expected degraded status, got %+v", body)
	}
	if body.Dependencies["chat_model"].Message == "" {
		t.Fatalf("expected actionable provider message, got %+v", body.Dependencies["chat_model"])
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
	cookie := findCookie(t, created, authpkg.SessionCookie)
	if !cookie.HttpOnly {
		t.Fatal("expected session cookie to be HTTP-only")
	}

	me := performJSONWithCookie(t, a, http.MethodGet, "/api/auth/me", "", cookie)
	if me.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, me.Code)
	}
	var auth authResponse
	decodeRecorder(t, me, &auth)
	if auth.User.Username != "admin" || auth.User.Role != authpkg.RoleAdmin {
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
	createUserForTest(t, a, "admin", "password123", authpkg.RoleAdmin)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"password123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, login.Code, login.Body.String())
	}
	cookie := findCookie(t, login, authpkg.SessionCookie)

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
	createUserForTest(t, a, "member", "password123", authpkg.RoleMember)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"member","password":"password123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, login.Code, login.Body.String())
	}
	cookie := findCookie(t, login, authpkg.SessionCookie)

	admin := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/status", "", cookie)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, admin.Code)
	}

	var body httpapi.APIError
	decodeRecorder(t, admin, &body)
	if body.Code != "forbidden" {
		t.Fatalf("expected stable forbidden code, got %+v", body)
	}
}

func TestAdminCanManageUsers(t *testing.T) {
	a := newTestApp(t)
	adminCookie := loginAs(t, a, "admin", authpkg.RoleAdmin)

	create := performJSONWithCookie(t, a, http.MethodPost, "/api/admin/users", `{"username":"Alice","password":"password123","role":"member"}`, adminCookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", create.Code, create.Body.String())
	}
	var created authResponse
	decodeRecorder(t, create, &created)
	if created.User.Username != "alice" || created.User.Role != authpkg.RoleMember {
		t.Fatalf("unexpected created user: %+v", created.User)
	}

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/users", "", adminCookie)
	if list.Code != http.StatusOK {
		t.Fatalf("list users: %d %s", list.Code, list.Body.String())
	}
	var listed adminUsersResponse
	decodeRecorder(t, list, &listed)
	if len(listed.Users) != 2 {
		t.Fatalf("expected two users, got %+v", listed.Users)
	}

	update := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/users/"+strconv.FormatInt(created.User.ID, 10), `{"username":"Alice2","password":"newpassword123","role":"admin"}`, adminCookie)
	if update.Code != http.StatusOK {
		t.Fatalf("update user: %d %s", update.Code, update.Body.String())
	}
	var updated authResponse
	decodeRecorder(t, update, &updated)
	if updated.User.Username != "alice2" || updated.User.Role != authpkg.RoleAdmin {
		t.Fatalf("unexpected updated user: %+v", updated.User)
	}
	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"alice2","password":"newpassword123"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("updated user could not log in: %d %s", login.Code, login.Body.String())
	}

	deleted := performJSONWithCookie(t, a, http.MethodDelete, "/api/admin/users/"+strconv.FormatInt(updated.User.ID, 10), "", adminCookie)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete user: %d %s", deleted.Code, deleted.Body.String())
	}
	missing := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/users/"+strconv.FormatInt(updated.User.ID, 10), `{"role":"member"}`, adminCookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted user to be missing, got %d %s", missing.Code, missing.Body.String())
	}
}

func TestAdminUserManagementGuardrails(t *testing.T) {
	a := newTestApp(t)
	admin := createUserForTest(t, a, "admin", "password123", authpkg.RoleAdmin)
	adminCookie := loginAsExisting(t, a, "admin", "password123")

	duplicate := performJSONWithCookie(t, a, http.MethodPost, "/api/admin/users", `{"username":"admin","password":"password123","role":"member"}`, adminCookie)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected duplicate conflict, got %d %s", duplicate.Code, duplicate.Body.String())
	}

	selfDemote := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/users/"+strconv.FormatInt(admin.ID, 10), `{"role":"member"}`, adminCookie)
	if selfDemote.Code != http.StatusBadRequest {
		t.Fatalf("expected self demote rejection, got %d %s", selfDemote.Code, selfDemote.Body.String())
	}

	selfDelete := performJSONWithCookie(t, a, http.MethodDelete, "/api/admin/users/"+strconv.FormatInt(admin.ID, 10), "", adminCookie)
	if selfDelete.Code != http.StatusBadRequest {
		t.Fatalf("expected self delete rejection, got %d %s", selfDelete.Code, selfDelete.Body.String())
	}

	memberCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	owned := createKnowledgeBaseForTest(t, a, memberCookie, "Owned")
	owner, err := a.store.FindUserByUsername(context.Background(), "owner")
	if err != nil {
		t.Fatalf("find owner: %v", err)
	}
	deleteOwner := performJSONWithCookie(t, a, http.MethodDelete, "/api/admin/users/"+strconv.FormatInt(owner.ID, 10), "", adminCookie)
	if deleteOwner.Code != http.StatusConflict {
		t.Fatalf("expected owned user delete conflict for kb %d, got %d %s", owned.ID, deleteOwner.Code, deleteOwner.Body.String())
	}
}

func TestInvalidLoginReturnsStructuredError(t *testing.T) {
	a := newTestApp(t)
	createUserForTest(t, a, "admin", "password123", authpkg.RoleAdmin)

	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`)
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, login.Code)
	}

	var body httpapi.APIError
	decodeRecorder(t, login, &body)
	if body.Code != "invalid_credentials" || body.Message == "" {
		t.Fatalf("expected structured error, got %+v", body)
	}
}

func TestAdminCanListMaskedProviderSettings(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", authpkg.RoleAdmin)

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
	cookie := loginAs(t, a, "member", authpkg.RoleMember)

	res := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/provider-settings", "", cookie)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestAdminCanSwitchProviderAndSecretIsMasked(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", authpkg.RoleAdmin)

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
	cookie := loginAs(t, a, "admin", authpkg.RoleAdmin)

	res := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/provider-settings/chat", `{
		"base_url":"file:///tmp/model",
		"model":"gpt-test"
	}`, cookie)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	var body httpapi.APIError
	decodeRecorder(t, res, &body)
	if body.Code != "provider_base_url_invalid" {
		t.Fatalf("expected provider_base_url_invalid, got %+v", body)
	}
}

func TestAdminObservabilityRecordsActivityAndDebugMode(t *testing.T) {
	a := newTestApp(t)
	adminCookie := loginAs(t, a, "admin", authpkg.RoleAdmin)
	createUserForTest(t, a, "member", "password123", authpkg.RoleMember)
	failed := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"member","password":"wrong"}`)
	if failed.Code != http.StatusUnauthorized {
		t.Fatalf("expected failed login status %d, got %d", http.StatusUnauthorized, failed.Code)
	}
	debug := performJSONWithCookie(t, a, http.MethodPut, "/api/admin/debug", `{"enabled":true}`, adminCookie)
	if debug.Code != http.StatusOK {
		t.Fatalf("expected debug status %d, got %d: %s", http.StatusOK, debug.Code, debug.Body.String())
	}
	activity := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/activity", "", adminCookie)
	if activity.Code != http.StatusOK {
		t.Fatalf("expected activity status %d, got %d: %s", http.StatusOK, activity.Code, activity.Body.String())
	}
	if !strings.Contains(activity.Body.String(), "login_failed") || !strings.Contains(activity.Body.String(), "debug_mode_changed") {
		t.Fatalf("expected activity events, got %s", activity.Body.String())
	}
}

func TestMetricsAndCorrelationID(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "admin", authpkg.RoleAdmin)
	if err := a.store.RecordMetric(context.Background(), "test_metric", 25*time.Millisecond, 2, map[string]any{"ok": true}); err != nil {
		t.Fatalf("record metric: %v", err)
	}
	metrics := performJSONWithCookie(t, a, http.MethodGet, "/api/admin/metrics", "", cookie)
	if metrics.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d: %s", http.StatusOK, metrics.Code, metrics.Body.String())
	}
	if !strings.Contains(metrics.Body.String(), "test_metric") {
		t.Fatalf("expected recorded metric, got %s", metrics.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("X-Request-ID", "test-correlation")
	res := httptest.NewRecorder()
	mux := http.NewServeMux()
	a.routes(mux)
	httpapi.WithCorrelation(mux).ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") != "test-correlation" {
		t.Fatalf("expected correlation header, got %q", res.Header().Get("X-Request-ID"))
	}
}

func TestMemberCanManageOwnPrivateKnowledgeBases(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Finance"}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, created.Code, created.Body.String())
	}
	var kb knowledgepkg.Response
	decodeRecorder(t, created, &kb)
	if kb.Name != "Finance" || kb.Visibility != visibilityPrivate || !kb.CanWrite {
		t.Fatalf("unexpected knowledge base: %+v", kb)
	}

	renamed := performJSONWithCookie(t, a, http.MethodPut, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), `{"name":"Finance 2026"}`, cookie)
	if renamed.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, renamed.Code, renamed.Body.String())
	}
	decodeRecorder(t, renamed, &kb)
	if kb.Name != "Finance 2026" {
		t.Fatalf("expected renamed knowledge base, got %+v", kb)
	}

	deleted := performJSONWithCookie(t, a, http.MethodDelete, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), "", cookie)
	if deleted.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, deleted.Code)
	}

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases", "", cookie)
	var body knowledgeBaseListResponse
	decodeRecorder(t, list, &body)
	if len(body.KnowledgeBases) != 0 {
		t.Fatalf("expected deleted knowledge base to be hidden, got %+v", body.KnowledgeBases)
	}
}

func TestMemberCannotReadOrMutateOtherPrivateKnowledgeBase(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	otherCookie := loginAs(t, a, "other", authpkg.RoleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Private"}`, ownerCookie)
	var kb knowledgepkg.Response
	decodeRecorder(t, created, &kb)

	read := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), "", otherCookie)
	if read.Code != http.StatusNotFound {
		t.Fatalf("expected private knowledge base to be hidden, got %d", read.Code)
	}
	update := performJSONWithCookie(t, a, http.MethodPut, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), `{"name":"Stolen"}`, otherCookie)
	if update.Code != http.StatusNotFound {
		t.Fatalf("expected private knowledge base mutation to be hidden, got %d", update.Code)
	}
}

func TestAdminCanMakeKnowledgeBasePublicAndMembersCanReadIt(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	adminCookie := loginAs(t, a, "admin", authpkg.RoleAdmin)
	readerCookie := loginAs(t, a, "reader", authpkg.RoleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Shared"}`, ownerCookie)
	var kb knowledgepkg.Response
	decodeRecorder(t, created, &kb)

	published := performJSONWithCookie(t, a, http.MethodPut, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), `{"name":"Shared","visibility":"public"}`, adminCookie)
	if published.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, published.Code, published.Body.String())
	}

	read := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), "", readerCookie)
	if read.Code != http.StatusOK {
		t.Fatalf("expected public knowledge base to be readable, got %d: %s", read.Code, read.Body.String())
	}
	decodeRecorder(t, read, &kb)
	if kb.Visibility != visibilityPublic || kb.CanWrite {
		t.Fatalf("expected public read-only knowledge base for reader, got %+v", kb)
	}
}

func TestMemberCannotMakeKnowledgeBasePublic(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Private"}`, cookie)
	var kb knowledgepkg.Response
	decodeRecorder(t, created, &kb)

	res := performJSONWithCookie(t, a, http.MethodPut, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), `{"name":"Private","visibility":"public"}`, cookie)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestUploadStoresDocumentMetadata(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")

	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("quarterly report"), false)
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, uploaded.Code, uploaded.Body.String())
	}
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)
	if doc.OriginalFilename != "report.pdf" || doc.DisplayName != "report.pdf" || doc.SizeBytes != int64(len("quarterly report")) {
		t.Fatalf("unexpected document metadata: %+v", doc)
	}
	if doc.SHA256 == "" || doc.Status != documentStatusPending {
		t.Fatalf("expected hash and pending status, got %+v", doc)
	}

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/documents", "", cookie)
	if list.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, list.Code)
	}
	var docs documentListResponse
	decodeRecorder(t, list, &docs)
	if len(docs.Documents) != 1 || docs.Documents[0].ID != doc.ID {
		t.Fatalf("expected uploaded document in list, got %+v", docs.Documents)
	}
}

func TestUploadDuplicateWarnsBeforeCreatingDocument(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")

	first := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("same content"), false)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, first.Code)
	}

	duplicate := uploadFile(t, a, cookie, kb.ID, "copy.pdf", "application/pdf", []byte("same content"), false)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, duplicate.Code, duplicate.Body.String())
	}
	var apiErr httpapi.APIError
	decodeRecorder(t, duplicate, &apiErr)
	if apiErr.Code != "duplicate_document" || apiErr.Details["duplicate"] == nil {
		t.Fatalf("expected duplicate details, got %+v", apiErr)
	}

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/documents", "", cookie)
	var docs documentListResponse
	decodeRecorder(t, list, &docs)
	if len(docs.Documents) != 1 {
		t.Fatalf("expected duplicate warning to avoid creating document, got %+v", docs.Documents)
	}

	confirmed := uploadFile(t, a, cookie, kb.ID, "copy.pdf", "application/pdf", []byte("same content"), true)
	if confirmed.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, confirmed.Code, confirmed.Body.String())
	}
}

func TestUploadRequiresKnowledgeBaseWriteAccess(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	otherCookie := loginAs(t, a, "other", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, ownerCookie, "Private")

	res := uploadFile(t, a, otherCookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected private knowledge base to be hidden, got %d", res.Code)
	}
}

func TestDuplicateDetectionDoesNotLeakOtherPrivateKnowledgeBases(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	otherCookie := loginAs(t, a, "other", authpkg.RoleMember)
	ownerKB := createKnowledgeBaseForTest(t, a, ownerCookie, "Owner")
	otherKB := createKnowledgeBaseForTest(t, a, otherCookie, "Other")

	first := uploadFile(t, a, ownerCookie, ownerKB.ID, "private.pdf", "application/pdf", []byte("same private content"), false)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, first.Code)
	}

	second := uploadFile(t, a, otherCookie, otherKB.ID, "other.pdf", "application/pdf", []byte("same private content"), false)
	if second.Code != http.StatusCreated {
		t.Fatalf("expected duplicate in another private knowledge base not to leak, got %d: %s", second.Code, second.Body.String())
	}
}

func TestIngestionJobStoresExtractedMarkdownArtifact(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = fakeDocumentClient(http.StatusOK, `{
		"schema_version":"v0.fake",
		"markdown":"# Extracted\n\nFake content.",
		"metadata":{"kind":"fake"},
		"warnings":[],
		"ocr":{"used":false},
		"source_anchors":[{"id":"page-1","kind":"page","label":"Page 1"}]
	}`)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)

	processed, err := a.processNextIngestionJob(context.Background())
	if err != nil {
		t.Fatalf("process ingestion: %v", err)
	}
	if !processed {
		t.Fatal("expected one ingestion job to be processed")
	}

	stored, err := a.store.FindDocumentByID(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if stored.Status != documentStatusReady {
		t.Fatalf("expected ready document, got %+v", stored)
	}

	preview := performJSONWithCookie(t, a, http.MethodGet, "/api/documents/"+strconv.FormatInt(doc.ID, 10)+"/extracted-markdown", "", cookie)
	if preview.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, preview.Code, preview.Body.String())
	}
	var body extractedMarkdownResponse
	decodeRecorder(t, preview, &body)
	if !strings.Contains(body.Markdown, "Fake content") {
		t.Fatalf("expected extracted Markdown, got %+v", body)
	}
}

func TestDocumentSearchIndexesExtractedText(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = fakeDocumentClient(http.StatusOK, `{
		"schema_version":"v0.fake",
		"markdown":"# Quarterly Plan\n\nRevenue expansion and hiring notes.",
		"metadata":{"kind":"fake"},
		"warnings":[],
		"ocr":{"used":false},
		"source_anchors":[{"id":"page-1","kind":"page","label":"Page 1"}]
	}`)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Searchable")
	uploaded := uploadFile(t, a, cookie, kb.ID, "plan.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)

	processed, err := a.processNextIngestionJob(context.Background())
	if err != nil {
		t.Fatalf("process ingestion: %v", err)
	}
	if !processed {
		t.Fatal("expected one ingestion job to be processed")
	}

	search := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/documents/search?q=revenue", "", cookie)
	if search.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, search.Code, search.Body.String())
	}
	var body documentListResponse
	decodeRecorder(t, search, &body)
	if len(body.Documents) != 1 || body.Documents[0].ID != doc.ID {
		t.Fatalf("expected indexed document search result, got %+v", body.Documents)
	}
}

func TestDocumentDeleteRemovesIndexedChunksFromVectorSearch(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = fakeDocumentClient(http.StatusOK, `{
		"schema_version":"v0.fake",
		"markdown":"# Alpha\n\nUnique tombstone retrieval text.",
		"metadata":{"kind":"fake"},
		"warnings":[],
		"ocr":{"used":false},
		"source_anchors":[{"id":"page-1","kind":"page","label":"Page 1"}]
	}`)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Delete")
	uploaded := uploadFile(t, a, cookie, kb.ID, "delete.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("process ingestion processed=%v err=%v", processed, err)
	}
	before, err := a.vectorIndex.Search(context.Background(), "tombstone retrieval", 5)
	if err != nil {
		t.Fatalf("vector search before delete: %v", err)
	}
	if len(before) == 0 || before[0].DocumentID != doc.ID {
		t.Fatalf("expected vector result before delete, got %+v", before)
	}

	deleted := performJSONWithCookie(t, a, http.MethodDelete, "/api/documents/"+strconv.FormatInt(doc.ID, 10), "", cookie)
	if deleted.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, deleted.Code, deleted.Body.String())
	}
	after, err := a.vectorIndex.Search(context.Background(), "tombstone retrieval", 5)
	if err != nil {
		t.Fatalf("vector search after delete: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected deleted chunks to be removed from vector search, got %+v", after)
	}
}

func TestVectorIndexPersistsWithoutStartupRebuild(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Persistent Vectors")
	insertIndexedDocumentForTest(t, a, kb.ID, "persist.pdf", "persistent vector evidence")

	reloaded, err := search.NewVectorIndex(a.embeddingFunc(), a.config.storageRoot)
	if err != nil {
		t.Fatalf("reload vector index: %v", err)
	}
	results, err := reloaded.Search(context.Background(), "persistent vector", 5)
	if err != nil {
		t.Fatalf("search persisted vector index: %v", err)
	}
	if len(results) == 0 || !strings.Contains(results[0].Content, "persistent vector evidence") {
		t.Fatalf("expected persisted vector result without rebuild, got %+v", results)
	}
}

func TestDocumentReprocessSupersedesOldChunksAfterSuccess(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = sequentialDocumentClient(t, []string{
		`{
			"schema_version":"v0.fake",
			"markdown":"# First\n\nOld searchable phrase.",
			"metadata":{"kind":"fake"},
			"warnings":[],
			"ocr":{"used":false},
			"source_anchors":[{"id":"page-1","kind":"page","label":"Page 1"}]
		}`,
		`{
			"schema_version":"v0.fake",
			"markdown":"# Second\n\nNew replacement phrase.",
			"metadata":{"kind":"fake"},
			"warnings":[],
			"ocr":{"used":false},
			"source_anchors":[{"id":"page-2","kind":"page","label":"Page 2"}]
		}`,
	})
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Reprocess")
	uploaded := uploadFile(t, a, cookie, kb.ID, "reprocess.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("first ingestion processed=%v err=%v", processed, err)
	}
	reprocess := performJSONWithCookie(t, a, http.MethodPost, "/api/documents/"+strconv.FormatInt(doc.ID, 10)+"/reprocess", `{}`, cookie)
	if reprocess.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, reprocess.Code, reprocess.Body.String())
	}
	pendingResults, err := a.vectorIndex.Search(context.Background(), "Old searchable", 5)
	if err != nil {
		t.Fatalf("pending reprocess vector search: %v", err)
	}
	if len(pendingResults) == 0 || !strings.Contains(pendingResults[0].Content, "Old searchable") {
		t.Fatalf("expected old chunks during pending reprocess, got %+v", pendingResults)
	}
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("second ingestion processed=%v err=%v", processed, err)
	}

	oldResults, err := a.vectorIndex.Search(context.Background(), "Old searchable", 5)
	if err != nil {
		t.Fatalf("old vector search: %v", err)
	}
	for _, result := range oldResults {
		if strings.Contains(result.Content, "Old searchable") {
			t.Fatalf("expected old chunks to be superseded, got %+v", oldResults)
		}
	}
	newResults, err := a.vectorIndex.Search(context.Background(), "New replacement", 5)
	if err != nil {
		t.Fatalf("new vector search: %v", err)
	}
	if len(newResults) == 0 || !strings.Contains(newResults[0].Content, "New replacement") {
		t.Fatalf("expected new chunks in vector index, got %+v", newResults)
	}
}

func TestKnowledgeBaseChatStreamsGroundedAnswer(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = fakeDocumentClient(http.StatusOK, `{
		"schema_version":"v0.fake",
		"markdown":"# Policy\n\nRemote work requires manager approval.",
		"metadata":{"kind":"fake"},
		"warnings":[],
		"ocr":{"used":false},
		"source_anchors":[{"id":"page-1","kind":"page","label":"Page 1"}]
	}`)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Policies")
	uploaded := uploadFile(t, a, cookie, kb.ID, "policy.pdf", "application/pdf", []byte("content"), false)
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", uploaded.Code, uploaded.Body.String())
	}
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("process ingestion processed=%v err=%v", processed, err)
	}

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"What is the remote work policy?"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["retrieval"]) == 0 {
		t.Fatalf("expected retrieval event, got %+v", events)
	}
	if !strings.Contains(joinDeltaText(t, events["delta"]), "retrieved Knowledge Base evidence") {
		t.Fatalf("expected streamed answer deltas, got %+v", events["delta"])
	}
	if len(events["citations"]) == 0 || !strings.Contains(events["citations"][0], "policy.pdf") {
		t.Fatalf("expected citation metadata, got %+v", events["citations"])
	}
	var start struct {
		SessionID string `json:"session_id"`
	}
	decodeJSONText(t, events["start"][0], &start)
	preview := performJSONWithCookie(t, a, http.MethodGet, "/api/chat-sessions/"+start.SessionID+"/citations/c1/preview", "", cookie)
	if preview.Code != http.StatusOK {
		t.Fatalf("expected preview status %d, got %d: %s", http.StatusOK, preview.Code, preview.Body.String())
	}
	var previewBody citationPreviewResponse
	decodeRecorder(t, preview, &previewBody)
	if previewBody.DocumentName != "policy.pdf" || !strings.Contains(previewBody.Text, "Remote work") || previewBody.OriginalDownloadURL == "" {
		t.Fatalf("unexpected citation preview: %+v", previewBody)
	}
	messages, err := a.store.ListChatMessages(context.Background(), start.SessionID, 10)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	if !chatMessagesContainCitation(messages, "c1") {
		t.Fatalf("expected persisted citation evidence, got %+v", messages)
	}
}

func TestChatSessionAPIsListDetailAndDeleteOwnedSessions(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Policies")
	insertIndexedDocumentForTest(t, a, kb.ID, "policy.pdf", "Remote work requires manager approval.")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"What is the remote work policy?"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected chat status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	sessionID := mustSessionIDFromEvents(t, events)

	list := performJSONWithCookie(t, a, http.MethodGet, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat-sessions", "", cookie)
	if list.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	var listBody chatSessionListResponse
	decodeRecorder(t, list, &listBody)
	if len(listBody.Sessions) != 1 || listBody.Sessions[0].ID != sessionID {
		t.Fatalf("unexpected session list: %+v", listBody)
	}

	detail := performJSONWithCookie(t, a, http.MethodGet, "/api/chat-sessions/"+sessionID, "", cookie)
	if detail.Code != http.StatusOK {
		t.Fatalf("expected detail status %d, got %d: %s", http.StatusOK, detail.Code, detail.Body.String())
	}
	var detailBody chatSessionDetailResponse
	decodeRecorder(t, detail, &detailBody)
	if detailBody.Session.ID != sessionID || len(detailBody.Messages) != 2 {
		t.Fatalf("unexpected session detail: %+v", detailBody)
	}
	if detailBody.Messages[0].Role != "user" || detailBody.Messages[1].Role != "assistant" {
		t.Fatalf("expected user and assistant messages, got %+v", detailBody.Messages)
	}
	if len(detailBody.Messages[1].Citations) == 0 {
		t.Fatalf("expected assistant citation metadata, got %+v", detailBody.Messages[1])
	}

	otherCookie := loginAs(t, a, "other", authpkg.RoleMember)
	otherDetail := performJSONWithCookie(t, a, http.MethodGet, "/api/chat-sessions/"+sessionID, "", otherCookie)
	if otherDetail.Code != http.StatusNotFound {
		t.Fatalf("expected other user detail status %d, got %d: %s", http.StatusNotFound, otherDetail.Code, otherDetail.Body.String())
	}
	otherDelete := performJSONWithCookie(t, a, http.MethodDelete, "/api/chat-sessions/"+sessionID, "", otherCookie)
	if otherDelete.Code != http.StatusNotFound {
		t.Fatalf("expected other user delete status %d, got %d: %s", http.StatusNotFound, otherDelete.Code, otherDelete.Body.String())
	}

	deleted := performJSONWithCookie(t, a, http.MethodDelete, "/api/chat-sessions/"+sessionID, "", cookie)
	if deleted.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d: %s", http.StatusOK, deleted.Code, deleted.Body.String())
	}
	missing := performJSONWithCookie(t, a, http.MethodGet, "/api/chat-sessions/"+sessionID, "", cookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted session to be missing, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestKnowledgeBaseChatContinuesCanonicalHarnessTranscript(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Policies")
	insertIndexedDocumentForTest(t, a, kb.ID, "policy.pdf", "Remote work requires manager approval.")

	first := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"What is the remote work policy?"}`, cookie)
	firstEvents := parseSSEEvents(t, first.Body.String())
	sessionID := mustSessionIDFromEvents(t, firstEvents)
	second := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"session_id":"`+sessionID+`","message":"What approval is required?"}`, cookie)
	if second.Code != http.StatusOK {
		t.Fatalf("second chat: %d %s", second.Code, second.Body.String())
	}
	secondEvents := parseSSEEvents(t, second.Body.String())
	if len(secondEvents["retrieval"]) == 0 || len(secondEvents["done"]) == 0 {
		t.Fatalf("expected retrieval and completion, got %+v", secondEvents)
	}
	transcript, err := a.store.ListAllChatMessages(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 8 {
		t.Fatalf("expected two four-message harness turns, got %#v", transcript)
	}
	if transcript[1].ToolCallsJSON == "[]" || transcript[2].Role != "tool" || transcript[6].Role != "tool" {
		t.Fatalf("canonical transcript not persisted: %#v", transcript)
	}
}

func TestKnowledgeBaseChatRetrievalScopeIsSelectedKnowledgeBase(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	firstKB := createKnowledgeBaseForTest(t, a, cookie, "First")
	secondKB := createKnowledgeBaseForTest(t, a, cookie, "Second")
	insertIndexedDocumentForTest(t, a, firstKB.ID, "first.pdf", "shared search term first-only")
	insertIndexedDocumentForTest(t, a, secondKB.ID, "second.pdf", "shared search term second-only")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(firstKB.ID, 10)+"/chat", `{"message":"shared search term"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["citations"]) == 0 {
		t.Fatalf("expected citations, got %+v", events)
	}
	if !strings.Contains(events["citations"][0], "first.pdf") || strings.Contains(events["citations"][0], "second.pdf") {
		t.Fatalf("expected citations scoped to selected KB, got %s", events["citations"][0])
	}
}

func TestKnowledgeBaseChatStreamingErrorWhenRetrievalFails(t *testing.T) {
	a := newTestApp(t)
	a.vectorIndex = nil
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Broken")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"Will retrieval fail?"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected streaming status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["error"]) == 0 || !strings.Contains(events["error"][0], "chat_error") {
		t.Fatalf("expected streaming error event, got %+v", events)
	}
}

func TestKnowledgeBaseChatAllowsAnswerWithoutRetrieval(t *testing.T) {
	a := newTestApp(t)
	current, err := a.store.FindProviderSetting(context.Background(), providerPurposeChat)
	if err != nil {
		t.Fatalf("load provider setting: %v", err)
	}
	current.Model = "fake-no-retrieval"
	if _, err := a.store.UpdateProviderSetting(context.Background(), current); err != nil {
		t.Fatalf("update provider setting: %v", err)
	}
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "No Retrieval")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"Answer without retrieval"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected streaming status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["error"]) != 0 {
		t.Fatalf("expected no streaming error, got %+v", events)
	}
	if !strings.Contains(joinDeltaText(t, events["delta"]), "Ungrounded answer.") {
		t.Fatalf("expected model answer to be streamed, got %+v", events["delta"])
	}
	if len(events["citations"]) == 0 || strings.Contains(events["citations"][0], "citation_id") {
		t.Fatalf("expected empty citations event, got %+v", events["citations"])
	}
	messages, err := a.store.ListChatMessages(context.Background(), mustSessionIDFromEvents(t, events), 10)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	if !chatMessagesContainText(messages, "Ungrounded answer.") {
		t.Fatalf("expected persisted model answer, got %+v", messages)
	}
}

func TestCitationPreviewAuthorization(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", authpkg.RoleMember)
	otherCookie := loginAs(t, a, "other", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, ownerCookie, "Private")
	insertIndexedDocumentForTest(t, a, kb.ID, "private.pdf", "private source text")
	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"private source"}`, ownerCookie)
	if res.Code != http.StatusOK {
		t.Fatalf("chat: %d %s", res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	var start struct {
		SessionID string `json:"session_id"`
	}
	decodeJSONText(t, events["start"][0], &start)

	preview := performJSONWithCookie(t, a, http.MethodGet, "/api/chat-sessions/"+start.SessionID+"/citations/c1/preview", "", otherCookie)
	if preview.Code != http.StatusNotFound {
		t.Fatalf("expected unauthorized preview to be hidden, got %d: %s", preview.Code, preview.Body.String())
	}
	download := performJSONWithCookie(t, a, http.MethodGet, "/api/documents/1/download", "", otherCookie)
	if download.Code != http.StatusNotFound {
		t.Fatalf("expected unauthorized download to be hidden, got %d", download.Code)
	}
}

func TestKnowledgeBaseChatUnsupportedAnswerHasNoCitations(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Empty")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"What is missing?"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["citations"]) == 0 {
		t.Fatalf("expected citations event, got %+v", events)
	}
	if strings.Contains(events["citations"][0], "citation_id") {
		t.Fatalf("expected no fake citations, got %s", events["citations"][0])
	}
	messages, err := a.store.ListChatMessages(context.Background(), mustSessionIDFromEvents(t, events), 10)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if chatMessagesContainCitation(messages, "c1") {
		t.Fatalf("expected no persisted citation evidence, got %+v", messages)
	}
	if !chatMessagesContainText(messages, unsupportedAnswerMessage()) {
		t.Fatalf("expected unsupported answer message, got %+v", messages)
	}
}

func TestIngestionFailureRetriesThenFails(t *testing.T) {
	a := newTestApp(t)
	a.config.documentURL = "http://document.test"
	a.httpClient = fakeDocumentClient(http.StatusInternalServerError, `{"error":"temporary"}`)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)

	for range 3 {
		processed, err := a.processNextIngestionJob(context.Background())
		if err != nil {
			t.Fatalf("process ingestion: %v", err)
		}
		if !processed {
			t.Fatal("expected ingestion job to be processed")
		}
	}

	stored, err := a.store.FindDocumentByID(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if stored.Status != documentStatusFailed || stored.ErrorCode != "document_extraction_failed" {
		t.Fatalf("expected failed document with extraction error, got %+v", stored)
	}
	job, err := a.store.FindLatestIngestionJobForDocument(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != ingestionJobFailed || job.Attempts != 3 {
		t.Fatalf("expected failed job after retries, got %+v", job)
	}
}

func TestIngestionCancellation(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", authpkg.RoleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc docpkg.Response
	decodeRecorder(t, uploaded, &doc)

	cancelled := performJSONWithCookie(t, a, http.MethodPost, "/api/documents/"+strconv.FormatInt(doc.ID, 10)+"/ingestion/cancel", `{}`, cookie)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, cancelled.Code, cancelled.Body.String())
	}
	processed, err := a.processNextIngestionJob(context.Background())
	if err != nil {
		t.Fatalf("process ingestion: %v", err)
	}
	if processed {
		t.Fatal("expected cancellation processing to skip extraction")
	}
	stored, err := a.store.FindDocumentByID(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if stored.Status != documentStatusCancelled {
		t.Fatalf("expected cancelled document, got %+v", stored)
	}
}

func TestOpenRouterProviderIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	baseURL := utils.Env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
	embeddingModel := utils.Env("OPENROUTER_EMBEDDING_MODEL", "qwen/qwen3-embedding-8b")
	chatModel := utils.Env("OPENROUTER_CHAT_MODEL", "poolside/laguna-xs.2")
	a := newTestApp(t)
	a.httpClient = &http.Client{Timeout: 2 * time.Minute}
	embedding, err := providers.OpenAICompatibleEmbedding(context.Background(), a.httpClient, domain.ProviderSetting{
		Purpose: providerPurposeEmbedding,
		BaseURL: baseURL,
		Model:   embeddingModel,
		APIKey:  apiKey,
	}, "short provider integration test", "")
	if err != nil {
		t.Fatalf("real embedding request: %v", err)
	}
	if len(embedding) == 0 {
		t.Fatal("expected embedding vector")
	}

	runner, err := agent.NewRunner(
		agent.WithAPIKey(apiKey),
		agent.WithBaseURL(baseURL),
		agent.WithModel(chatModel),
		agent.WithHTTPClient(a.chatClient()),
		agent.WithMaxTurns(1),
	)
	if err != nil {
		t.Fatalf("create real chat runner: %v", err)
	}
	snapshot, err := runner.RunTurn(context.Background(), agent.RunSnapshot{}, []agent.Message{{Role: agent.RoleUser, Content: "Reply with exactly: ok"}})
	if err != nil {
		t.Fatalf("real harness chat request: %v", err)
	}
	if len(snapshot.Transcript) == 0 || strings.TrimSpace(snapshot.Transcript[len(snapshot.Transcript)-1].Content) == "" {
		t.Fatalf("expected harness assistant response, got %+v", snapshot.Transcript)
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := testConfig(t)
	if err := store.EnsureProviderDefaults(context.Background(), cfg.defaultProviders); err != nil {
		t.Fatalf("seed provider defaults: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	a := &app{
		startedAt:        time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		config:           cfg,
		store:            store,
		chunkingStrategy: ingestionpkg.MarkdownChunkingStrategy{},
		activeChats:      chatpkg.NewCancelRegistry(),
	}
	vectorIndex, err := search.NewVectorIndex(a.embeddingFunc(), cfg.storageRoot)
	if err != nil {
		t.Fatalf("create vector index: %v", err)
	}
	a.vectorIndex = vectorIndex
	return a
}

func testConfig(t *testing.T) config {
	t.Helper()
	return config{
		documentURL:   "http://document:8081",
		ocrURL:        "http://ocr:8082",
		fakeProviders: true,
		storageRoot:   filepath.Join(t.TempDir(), "files"),
		defaultProviders: map[string]domain.ProviderSetting{
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

func createUserForTest(t *testing.T, a *app, username, password, role string) domain.User {
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
	return loginAsExisting(t, a, username, "password123")
}

func loginAsExisting(t *testing.T, a *app, username, password string) *http.Cookie {
	t.Helper()
	login := performJSON(t, a, http.MethodPost, "/api/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login as %s: %d %s", username, login.Code, login.Body.String())
	}
	return findCookie(t, login, authpkg.SessionCookie)
}

func createKnowledgeBaseForTest(t *testing.T, a *app, cookie *http.Cookie, name string) knowledgepkg.Response {
	t.Helper()
	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"`+name+`"}`, cookie)
	if res.Code != http.StatusCreated {
		t.Fatalf("create knowledge base: %d %s", res.Code, res.Body.String())
	}
	var kb knowledgepkg.Response
	decodeRecorder(t, res, &kb)
	return kb
}

func uploadFile(t *testing.T, a *app, cookie *http.Cookie, knowledgeBaseID int64, filename, contentType string, content []byte, confirmDuplicate bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	path := "/api/knowledge-bases/" + strconv.FormatInt(knowledgeBaseID, 10) + "/documents/upload"
	if confirmDuplicate {
		path += "?confirm_duplicate=true"
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()

	mux := http.NewServeMux()
	a.routes(mux)
	mux.ServeHTTP(res, req)
	return res
}

func insertIndexedDocumentForTest(t *testing.T, a *app, knowledgeBaseID int64, filename, content string) {
	t.Helper()
	current := createUserForTest(t, a, "owner-"+filename, "password123", authpkg.RoleMember)
	doc, err := a.store.CreateDocument(context.Background(), domain.DocumentRecord{
		KnowledgeBaseID:  knowledgeBaseID,
		OwnerID:          current.ID,
		OriginalFilename: filename,
		DisplayName:      filename,
		ContentType:      "application/pdf",
		SizeBytes:        int64(len(content)),
		SHA256:           filename,
		StorageKey:       filepath.Join("documents", filename, "original"),
		Status:           documentStatusReady,
	})
	if err != nil {
		t.Fatalf("create indexed test document: %v", err)
	}
	job, err := a.store.CreateIngestionJob(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("create ingestion job: %v", err)
	}
	if err := a.store.CompleteIngestionJob(context.Background(), job, doc, domain.DocumentVersion{
		DocumentID:         doc.ID,
		MarkdownStorageKey: filepath.Join("documents", filename, "extracted.md"),
		SchemaVersion:      "v0.test",
		MetadataJSON:       "{}",
		EmbeddingModel:     "fake-embedding",
	}, []domain.DocumentChunk{{
		Content:          content,
		SourceAnchorJSON: `{"id":"page-1","kind":"page","label":"Page 1"}`,
		TokenCount:       ingestionpkg.EstimatedTokenCount(content),
	}}); err != nil {
		t.Fatalf("complete ingestion: %v", err)
	}
	if err := a.rebuildVectorIndex(context.Background()); err != nil {
		t.Fatalf("rebuild vector index: %v", err)
	}
}

func parseSSEEvents(t *testing.T, stream string) map[string][]string {
	t.Helper()
	events := make(map[string][]string)
	var eventName string
	for _, line := range strings.Split(stream, "\n") {
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}
		if strings.HasPrefix(line, "data: ") && eventName != "" {
			events[eventName] = append(events[eventName], strings.TrimSpace(strings.TrimPrefix(line, "data: ")))
		}
	}
	return events
}

func joinDeltaText(t *testing.T, deltas []string) string {
	t.Helper()
	var out strings.Builder
	for _, delta := range deltas {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(delta), &payload); err != nil {
			t.Fatalf("decode delta: %v", err)
		}
		out.WriteString(payload.Text)
	}
	return out.String()
}

func decodeJSONText(t *testing.T, text string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), target); err != nil {
		t.Fatalf("decode JSON text: %v", err)
	}
}

func mustSessionIDFromEvents(t *testing.T, events map[string][]string) string {
	t.Helper()
	if len(events["start"]) == 0 {
		t.Fatalf("missing start event: %+v", events)
	}
	var start struct {
		SessionID string `json:"session_id"`
	}
	decodeJSONText(t, events["start"][0], &start)
	if start.SessionID == "" {
		t.Fatalf("missing session id in start event: %+v", events["start"])
	}
	return start.SessionID
}

func chatMessagesContainCitation(messages []domain.ChatMessage, citationID string) bool {
	for _, msg := range messages {
		var metadata struct {
			Citations []chatpkg.RetrievalEvidence `json:"citations"`
		}
		if json.Unmarshal([]byte(msg.Metadata), &metadata) != nil {
			continue
		}
		for _, citation := range metadata.Citations {
			if citation.CitationID == citationID {
				return true
			}
		}
	}
	return false
}

func chatMessagesContainText(messages []domain.ChatMessage, text string) bool {
	for _, msg := range messages {
		if strings.Contains(msg.Content, text) {
			return true
		}
	}
	return false
}

func fakeDocumentClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/extract" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":"not_found"}`)),
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: status,
			Status:     strconv.Itoa(status) + " " + http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
}

func sequentialDocumentClient(t *testing.T, bodies []string) *http.Client {
	t.Helper()
	var index int
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/extract" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":"not_found"}`)),
				Request:    r,
			}, nil
		}
		if index >= len(bodies) {
			t.Fatalf("unexpected extraction request %d", index+1)
		}
		body := bodies[index]
		index++
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
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
