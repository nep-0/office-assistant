package main

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

func TestMemberCanManageOwnPrivateKnowledgeBases(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", roleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Finance"}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, created.Code, created.Body.String())
	}
	var kb knowledgeBaseResponse
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
	ownerCookie := loginAs(t, a, "owner", roleMember)
	otherCookie := loginAs(t, a, "other", roleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Private"}`, ownerCookie)
	var kb knowledgeBaseResponse
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
	ownerCookie := loginAs(t, a, "owner", roleMember)
	adminCookie := loginAs(t, a, "admin", roleAdmin)
	readerCookie := loginAs(t, a, "reader", roleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Shared"}`, ownerCookie)
	var kb knowledgeBaseResponse
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
	cookie := loginAs(t, a, "member", roleMember)

	created := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"Private"}`, cookie)
	var kb knowledgeBaseResponse
	decodeRecorder(t, created, &kb)

	res := performJSONWithCookie(t, a, http.MethodPut, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10), `{"name":"Private","visibility":"public"}`, cookie)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestUploadStoresDocumentMetadata(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")

	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("quarterly report"), false)
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, uploaded.Code, uploaded.Body.String())
	}
	var doc documentResponse
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")

	first := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("same content"), false)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, first.Code)
	}

	duplicate := uploadFile(t, a, cookie, kb.ID, "copy.pdf", "application/pdf", []byte("same content"), false)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, duplicate.Code, duplicate.Body.String())
	}
	var apiErr apiError
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
	ownerCookie := loginAs(t, a, "owner", roleMember)
	otherCookie := loginAs(t, a, "other", roleMember)
	kb := createKnowledgeBaseForTest(t, a, ownerCookie, "Private")

	res := uploadFile(t, a, otherCookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected private knowledge base to be hidden, got %d", res.Code)
	}
}

func TestDuplicateDetectionDoesNotLeakOtherPrivateKnowledgeBases(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", roleMember)
	otherCookie := loginAs(t, a, "other", roleMember)
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
	decodeRecorder(t, uploaded, &doc)

	processed, err := a.processNextIngestionJob(context.Background())
	if err != nil {
		t.Fatalf("process ingestion: %v", err)
	}
	if !processed {
		t.Fatal("expected one ingestion job to be processed")
	}

	stored, err := a.store.findDocumentByID(context.Background(), doc.ID)
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Searchable")
	uploaded := uploadFile(t, a, cookie, kb.ID, "plan.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Delete")
	uploaded := uploadFile(t, a, cookie, kb.ID, "delete.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
	decodeRecorder(t, uploaded, &doc)
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("process ingestion processed=%v err=%v", processed, err)
	}
	before, err := a.vectorIndex.search(context.Background(), "tombstone retrieval", 5)
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
	after, err := a.vectorIndex.search(context.Background(), "tombstone retrieval", 5)
	if err != nil {
		t.Fatalf("vector search after delete: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected deleted chunks to be removed from vector search, got %+v", after)
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Reprocess")
	uploaded := uploadFile(t, a, cookie, kb.ID, "reprocess.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
	decodeRecorder(t, uploaded, &doc)
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("first ingestion processed=%v err=%v", processed, err)
	}
	reprocess := performJSONWithCookie(t, a, http.MethodPost, "/api/documents/"+strconv.FormatInt(doc.ID, 10)+"/reprocess", `{}`, cookie)
	if reprocess.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, reprocess.Code, reprocess.Body.String())
	}
	pendingResults, err := a.vectorIndex.search(context.Background(), "Old searchable", 5)
	if err != nil {
		t.Fatalf("pending reprocess vector search: %v", err)
	}
	if len(pendingResults) == 0 || !strings.Contains(pendingResults[0].Content, "Old searchable") {
		t.Fatalf("expected old chunks during pending reprocess, got %+v", pendingResults)
	}
	if processed, err := a.processNextIngestionJob(context.Background()); err != nil || !processed {
		t.Fatalf("second ingestion processed=%v err=%v", processed, err)
	}

	oldResults, err := a.vectorIndex.search(context.Background(), "Old searchable", 5)
	if err != nil {
		t.Fatalf("old vector search: %v", err)
	}
	for _, result := range oldResults {
		if strings.Contains(result.Content, "Old searchable") {
			t.Fatalf("expected old chunks to be superseded, got %+v", oldResults)
		}
	}
	newResults, err := a.vectorIndex.search(context.Background(), "New replacement", 5)
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
	cookie := loginAs(t, a, "member", roleMember)
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
	messages, err := a.store.listChatMessages(context.Background(), start.SessionID, 10)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	if !chatMessagesContainCitation(messages, "c1") {
		t.Fatalf("expected persisted citation evidence, got %+v", messages)
	}
}

func TestKnowledgeBaseChatRetrievalScopeIsSelectedKnowledgeBase(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", roleMember)
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
	cookie := loginAs(t, a, "member", roleMember)
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

func TestKnowledgeBaseChatRejectsAnswerWithoutRetrieval(t *testing.T) {
	a := newTestApp(t)
	current, err := a.store.findProviderSetting(context.Background(), providerPurposeChat)
	if err != nil {
		t.Fatalf("load provider setting: %v", err)
	}
	current.Model = "fake-no-retrieval"
	if _, err := a.store.updateProviderSetting(context.Background(), current); err != nil {
		t.Fatalf("update provider setting: %v", err)
	}
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "No Retrieval")

	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases/"+strconv.FormatInt(kb.ID, 10)+"/chat", `{"message":"Answer without retrieval"}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("expected streaming status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	events := parseSSEEvents(t, res.Body.String())
	if len(events["error"]) == 0 || !strings.Contains(events["error"][0], "retrieval_required") {
		t.Fatalf("expected retrieval_required error, got %+v", events)
	}
}

func TestCitationPreviewAuthorization(t *testing.T) {
	a := newTestApp(t)
	ownerCookie := loginAs(t, a, "owner", roleMember)
	otherCookie := loginAs(t, a, "other", roleMember)
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
	cookie := loginAs(t, a, "member", roleMember)
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
	messages, err := a.store.listChatMessages(context.Background(), mustSessionIDFromEvents(t, events), 10)
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
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
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

	stored, err := a.store.findDocumentByID(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if stored.Status != documentStatusFailed || stored.ErrorCode != "document_extraction_failed" {
		t.Fatalf("expected failed document with extraction error, got %+v", stored)
	}
	job, err := a.store.findLatestIngestionJobForDocument(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != ingestionJobFailed || job.Attempts != 3 {
		t.Fatalf("expected failed job after retries, got %+v", job)
	}
}

func TestIngestionCancellation(t *testing.T) {
	a := newTestApp(t)
	cookie := loginAs(t, a, "member", roleMember)
	kb := createKnowledgeBaseForTest(t, a, cookie, "Uploads")
	uploaded := uploadFile(t, a, cookie, kb.ID, "report.pdf", "application/pdf", []byte("content"), false)
	var doc documentResponse
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
	stored, err := a.store.findDocumentByID(context.Background(), doc.ID)
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
	baseURL := env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
	embeddingModel := env("OPENROUTER_EMBEDDING_MODEL", "qwen/qwen3-embedding-8b")
	chatModel := env("OPENROUTER_CHAT_MODEL", "poolside/laguna-xs.2")
	a := newTestApp(t)
	a.httpClient = &http.Client{Timeout: 2 * time.Minute}
	embedding, err := a.openAICompatibleEmbedding(context.Background(), providerSetting{
		Purpose: providerPurposeEmbedding,
		BaseURL: baseURL,
		Model:   embeddingModel,
		APIKey:  apiKey,
	}, "short provider integration test")
	if err != nil {
		t.Fatalf("real embedding request: %v", err)
	}
	if len(embedding) == 0 {
		t.Fatal("expected embedding vector")
	}

	payload := map[string]any{
		"model": chatModel,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly: ok"},
		},
		"max_tokens": 8,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal chat payload: %v", err)
	}
	endpoint, err := joinProviderPath(baseURL, "chat/completions")
	if err != nil {
		t.Fatalf("chat endpoint: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new chat request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("real chat request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		t.Fatalf("chat provider returned %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&decoded); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(decoded.Choices) == 0 {
		t.Fatalf("expected at least one chat choice, got %+v", decoded.Choices)
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := testConfig(t)
	if err := store.ensureProviderDefaults(context.Background(), cfg.defaultProviders); err != nil {
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
		chunkingStrategy: markdownChunkingStrategy{},
		activeChats:      make(map[string]context.CancelFunc),
	}
	vectorIndex, err := newVectorIndex(a.embeddingFunc())
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

func createKnowledgeBaseForTest(t *testing.T, a *app, cookie *http.Cookie, name string) knowledgeBaseResponse {
	t.Helper()
	res := performJSONWithCookie(t, a, http.MethodPost, "/api/knowledge-bases", `{"name":"`+name+`"}`, cookie)
	if res.Code != http.StatusCreated {
		t.Fatalf("create knowledge base: %d %s", res.Code, res.Body.String())
	}
	var kb knowledgeBaseResponse
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
	current := createUserForTest(t, a, "owner-"+filename, "password123", roleMember)
	doc, err := a.store.createDocument(context.Background(), documentRecord{
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
	job, err := a.store.createIngestionJob(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("create ingestion job: %v", err)
	}
	if err := a.store.completeIngestionJob(context.Background(), job, doc, documentVersion{
		DocumentID:         doc.ID,
		MarkdownStorageKey: filepath.Join("documents", filename, "extracted.md"),
		SchemaVersion:      "v0.test",
		MetadataJSON:       "{}",
		EmbeddingModel:     "fake-embedding",
	}, []documentChunk{{
		Content:          content,
		SourceAnchorJSON: `{"id":"page-1","kind":"page","label":"Page 1"}`,
		TokenCount:       estimatedTokenCount(content),
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

func chatMessagesContainCitation(messages []chatMessage, citationID string) bool {
	for _, msg := range messages {
		var metadata struct {
			Citations []retrievalEvidence `json:"citations"`
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

func chatMessagesContainText(messages []chatMessage, text string) bool {
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
