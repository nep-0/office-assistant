package documents

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"office-assistant/backend/auth"
	"office-assistant/backend/domain"
	"office-assistant/backend/ingestion"
	"office-assistant/backend/knowledge"
	"office-assistant/backend/store"
)

func TestLifecycleUploadCreatesDocumentAndJob(t *testing.T) {
	lifecycle, owner, kb := newTestLifecycle(t)

	doc, err := lifecycle.Upload(context.Background(), UploadInput{
		Current:          owner,
		KnowledgeBase:    kb,
		File:             strings.NewReader("quarterly report"),
		OriginalFilename: "report.pdf",
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if doc.Status != StatusPending || doc.DisplayName != "report.pdf" || doc.SHA256 == "" {
		t.Fatalf("unexpected document: %+v", doc)
	}
	job, err := lifecycle.Store.FindLatestIngestionJobForDocument(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("find ingestion job: %v", err)
	}
	if job.Status != "pending" {
		t.Fatalf("expected pending ingestion job, got %+v", job)
	}
}

func TestLifecycleUploadDuplicateReturnsDuplicateBeforeCreate(t *testing.T) {
	lifecycle, owner, kb := newTestLifecycle(t)

	_, err := lifecycle.Upload(context.Background(), UploadInput{
		Current:          owner,
		KnowledgeBase:    kb,
		File:             strings.NewReader("same content"),
		OriginalFilename: "report.pdf",
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("initial upload: %v", err)
	}

	_, err = lifecycle.Upload(context.Background(), UploadInput{
		Current:          owner,
		KnowledgeBase:    kb,
		File:             strings.NewReader("same content"),
		OriginalFilename: "copy.pdf",
		ContentType:      "application/pdf",
	})
	var duplicate DuplicateError
	if !errors.As(err, &duplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if duplicate.Duplicate.DisplayName != "report.pdf" {
		t.Fatalf("unexpected duplicate document: %+v", duplicate.Duplicate)
	}
	docs, err := lifecycle.Store.ListDocumentsForKnowledgeBase(context.Background(), kb.ID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected duplicate warning to avoid creating a second document, got %+v", docs)
	}
}

func TestLifecycleDeleteEnforcesWritePermissionAndRefreshesIndex(t *testing.T) {
	lifecycle, owner, kb := newTestLifecycle(t)
	other, err := lifecycle.Store.CreateUser(context.Background(), "other", "hash", auth.RoleMember)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	doc, err := lifecycle.Upload(context.Background(), UploadInput{
		Current:          owner,
		KnowledgeBase:    kb,
		File:             strings.NewReader("delete me"),
		OriginalFilename: "delete.pdf",
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	err = lifecycle.Delete(context.Background(), other, doc)
	var lifecycleErr LifecycleError
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Code != "forbidden" {
		t.Fatalf("expected forbidden lifecycle error, got %v", err)
	}
	index := lifecycle.RetrievalIndex.(*fakeRetrievalIndex)
	if index.refreshes != 0 {
		t.Fatalf("index refreshed after forbidden delete")
	}

	if err := lifecycle.Delete(context.Background(), owner, doc); err != nil {
		t.Fatalf("delete as owner: %v", err)
	}
	if index.refreshes != 1 {
		t.Fatalf("expected one index refresh, got %d", index.refreshes)
	}
}

func newTestLifecycle(t *testing.T) (Lifecycle, domain.User, domain.KnowledgeBase) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	owner, err := st.CreateUser(context.Background(), "owner", "hash", auth.RoleMember)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	kb, err := st.CreateKnowledgeBase(context.Background(), owner.ID, "Lifecycle", knowledge.VisibilityPrivate)
	if err != nil {
		t.Fatalf("create knowledge base: %v", err)
	}
	token := 0
	lifecycle := Lifecycle{
		Store: st,
		Storage: LocalStorage{
			Root: filepath.Join(t.TempDir(), "files"),
			NewToken: func() (string, error) {
				token++
				return "token-" + strconv.Itoa(token), nil
			},
		},
		ChunkingStrategy: ingestion.MarkdownChunkingStrategy{},
		RetrievalIndex:   &fakeRetrievalIndex{},
		EmbeddingPurpose: "embedding",
	}
	return lifecycle, owner, kb
}

type fakeRetrievalIndex struct {
	refreshes int
}

func (idx *fakeRetrievalIndex) Preflight(context.Context, []domain.DocumentChunk) error {
	return nil
}

func (idx *fakeRetrievalIndex) Refresh(context.Context) error {
	idx.refreshes++
	return nil
}
