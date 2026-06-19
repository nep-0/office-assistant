package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"office-assistant/internal/document"
	"office-assistant/internal/storage"
)

func TestProviderSettingsAreMasked(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	token := bootstrapAdmin(t, handler)

	body := `{
		"embedding": {"kind":"cloud","base_url":"https://example.test","model":"embed-test","api_key":"embed-secret"},
		"chat": {"kind":"cloud","base_url":"https://example.test","model":"chat-test","api_key":"chat-secret"}
	}`
	put := httptest.NewRequest(http.MethodPut, "/api/provider-settings", strings.NewReader(body))
	put.Header.Set("Authorization", "Bearer "+token)
	putRecorder := httptest.NewRecorder()
	handler.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("expected put status 200, got %d: %s", putRecorder.Code, putRecorder.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/provider-settings", nil)
	get.Header.Set("Authorization", "Bearer "+token)
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("expected get status 200, got %d: %s", getRecorder.Code, getRecorder.Body.String())
	}

	var settings storage.ProviderSettings
	if err := json.NewDecoder(getRecorder.Body).Decode(&settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if settings.Embedding.APIKey != "********" {
		t.Fatalf("expected masked embedding key, got %q", settings.Embedding.APIKey)
	}
	if settings.Chat.APIKey != "********" {
		t.Fatalf("expected masked chat key, got %q", settings.Chat.APIKey)
	}
}

func TestHealthReportsDefaultProviders(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected ok health response, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"kind":"mock"`) {
		t.Fatalf("expected mock provider summary, got %s", recorder.Body.String())
	}
}

func TestChatStreamReturnsTokensAndFinalCitation(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	token := bootstrapAdmin(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"What can the assistant do?"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	events := parseSSE(t, recorder.Body)
	if len(events["token"]) == 0 {
		t.Fatalf("expected token events, got %#v", events)
	}
	finalEvents := events["final"]
	if len(finalEvents) != 1 {
		t.Fatalf("expected one final event, got %#v", finalEvents)
	}

	var final struct {
		Answer    string `json:"answer"`
		Citations []struct {
			SourceFileName string `json:"source_file_name"`
			Preview        string `json:"preview"`
		} `json:"citations"`
	}
	if err := json.Unmarshal([]byte(finalEvents[0]), &final); err != nil {
		t.Fatalf("decode final event: %v", err)
	}
	if final.Answer == "" {
		t.Fatal("expected final answer")
	}
	if len(final.Citations) != 1 {
		t.Fatalf("expected one citation, got %#v", final.Citations)
	}
	if final.Citations[0].SourceFileName != "spike-source.pdf" {
		t.Fatalf("unexpected citation source: %#v", final.Citations[0])
	}
}

func TestKnowledgeBaseMembershipFiltersUserList(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	adminToken := bootstrapAdmin(t, handler)

	user := createUser(t, handler, adminToken, "alice", "user")
	kb := createKnowledgeBase(t, handler, adminToken, "Finance")

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge-bases", nil)
	req.Header.Set("Authorization", "Bearer "+login(t, handler, "alice", "password"))
	before := httptest.NewRecorder()
	handler.ServeHTTP(before, req)
	if before.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", before.Code, before.Body.String())
	}
	if strings.Contains(before.Body.String(), "Finance") {
		t.Fatalf("expected unassigned user not to see knowledge base, got %s", before.Body.String())
	}

	addMember := httptest.NewRequest(http.MethodPost, "/api/knowledge-bases/"+kb.ID+"/members", strings.NewReader(`{"user_id":"`+user.ID+`"}`))
	addMember.Header.Set("Authorization", "Bearer "+adminToken)
	addMemberRecorder := httptest.NewRecorder()
	handler.ServeHTTP(addMemberRecorder, addMember)
	if addMemberRecorder.Code != http.StatusCreated {
		t.Fatalf("expected add member status 201, got %d: %s", addMemberRecorder.Code, addMemberRecorder.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/knowledge-bases", nil)
	req.Header.Set("Authorization", "Bearer "+login(t, handler, "alice", "password"))
	after := httptest.NewRecorder()
	handler.ServeHTTP(after, req)
	if after.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", after.Code, after.Body.String())
	}
	if !strings.Contains(after.Body.String(), "Finance") {
		t.Fatalf("expected assigned user to see knowledge base, got %s", after.Body.String())
	}
}

func TestProviderSettingsRequireAdmin(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	adminToken := bootstrapAdmin(t, handler)
	createUser(t, handler, adminToken, "bob", "user")
	userToken := login(t, handler, "bob", "password")

	req := httptest.NewRequest(http.MethodGet, "/api/provider-settings", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUploadDocumentStoresAndProcessesChunks(t *testing.T) {
	processor := fakeProcessor{
		chunks: []document.Chunk{
			{
				ID:               "chunk_test_1",
				Content:          "The uploaded document can be processed into chunks.",
				SourceFileName:   "sample.txt",
				ChunkIndex:       0,
				ContentType:      "text",
				TokenOrCharCount: 53,
				Metadata:         map[string]any{"source": "test"},
			},
		},
	}
	handler, cleanup := newTestHandlerWithProcessor(t, processor)
	defer cleanup()
	adminToken := bootstrapAdmin(t, handler)
	kb := createKnowledgeBase(t, handler, adminToken, "Policies")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "sample.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("sample office policy")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/knowledge-bases/"+kb.ID+"/documents", &body)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected upload status 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/knowledge-bases/"+kb.ID+"/documents", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		listRecorder := httptest.NewRecorder()
		handler.ServeHTTP(listRecorder, req)
		if listRecorder.Code != http.StatusOK {
			t.Fatalf("expected list status 200, got %d: %s", listRecorder.Code, listRecorder.Body.String())
		}

		var docs []storage.Document
		if err := json.NewDecoder(listRecorder.Body).Decode(&docs); err != nil {
			t.Fatalf("decode documents: %v", err)
		}
		if len(docs) == 1 && docs[0].Status == "indexed" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("document did not become indexed before deadline: %#v", docs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newTestHandler(t *testing.T) (http.Handler, func()) {
	return newTestHandlerWithProcessor(t, nil)
}

func newTestHandlerWithProcessor(t *testing.T, processor document.Processor) (http.Handler, func()) {
	t.Helper()

	store, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	handler := New(Options{
		Store:     store,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		JWTSecret: "test-secret",
		UploadDir: t.TempDir(),
		Processor: processor,
	})
	return handler, func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}

type fakeProcessor struct {
	chunks []document.Chunk
	err    error
}

func (p fakeProcessor) Process(ctx context.Context, input document.ProcessInput) ([]document.Chunk, error) {
	if p.err != nil {
		return nil, p.err
	}
	chunks := make([]document.Chunk, 0, len(p.chunks))
	for _, chunk := range p.chunks {
		chunk.DocumentID = input.DocumentID
		chunk.KnowledgeBaseID = input.KnowledgeBaseID
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func parseSSE(t *testing.T, body *bytes.Buffer) map[string][]string {
	t.Helper()

	events := map[string][]string{}
	scanner := bufio.NewScanner(body)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if event == "" {
				t.Fatalf("data line without event: %q", line)
			}
			events[event] = append(events[event], strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan sse: %v", err)
	}
	return events
}

func bootstrapAdmin(t *testing.T, handler http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", strings.NewReader(`{"username":"admin","password":"password"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap status 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("expected bootstrap token")
	}
	return response.Token
}

func login(t *testing.T, handler http.Handler, username, password string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("expected login token")
	}
	return response.Token
}

func createUser(t *testing.T, handler http.Handler, adminToken, username, role string) publicUserResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"username":"`+username+`","password":"password","role":"`+role+`"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected create user status 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var user publicUserResponse
	if err := json.NewDecoder(recorder.Body).Decode(&user); err != nil {
		t.Fatalf("decode create user response: %v", err)
	}
	return user
}

func createKnowledgeBase(t *testing.T, handler http.Handler, adminToken, name string) storage.KnowledgeBase {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/knowledge-bases", strings.NewReader(`{"name":"`+name+`"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected create knowledge base status 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var kb storage.KnowledgeBase
	if err := json.NewDecoder(recorder.Body).Decode(&kb); err != nil {
		t.Fatalf("decode create knowledge base response: %v", err)
	}
	return kb
}
