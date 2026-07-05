package documents

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"office-assistant/backend/domain"
	"office-assistant/backend/ingestion"
	"office-assistant/backend/store"
)

type LocalStorage struct {
	Root     string
	NewToken func() (string, error)
}

func (s LocalStorage) WriteUploadTemp(file io.Reader) (string, string, int64, error) {
	return WriteUploadTemp(s.Root, file)
}

func (s LocalStorage) PrepareStoragePath() (string, string, error) {
	token, err := s.NewToken()
	if err != nil {
		return "", "", err
	}
	return PrepareStoragePath(s.Root, token)
}

func (s LocalStorage) MoveTemp(tempPath, finalPath string) error {
	return os.Rename(tempPath, finalPath)
}

func (s LocalStorage) Remove(path string) error {
	return os.Remove(path)
}

func (s LocalStorage) WriteExtractedMarkdown(doc domain.DocumentRecord, markdown string) (string, error) {
	return ingestion.WriteExtractedMarkdown(s.Root, doc, markdown)
}

func (s LocalStorage) Read(storageKey string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Root, storageKey))
}

type DocumentExtractor struct {
	Client      *http.Client
	DocumentURL string
	StorageRoot string
}

func (e DocumentExtractor) Extract(ctx context.Context, doc domain.DocumentRecord) (ingestion.ExtractionPackage, error) {
	return ingestion.ExtractDocument(ctx, e.Client, e.DocumentURL, e.StorageRoot, doc)
}

type ChunkVectorIndex interface {
	Preflight(ctx context.Context, chunks []domain.DocumentChunk) error
	Rebuild(ctx context.Context, chunks []domain.DocumentChunk) error
}

type StoreBackedRetrievalIndex struct {
	Store *store.Store
	Index ChunkVectorIndex
}

func (idx StoreBackedRetrievalIndex) Preflight(ctx context.Context, chunks []domain.DocumentChunk) error {
	if idx.Index == nil {
		return nil
	}
	return idx.Index.Preflight(ctx, chunks)
}

func (idx StoreBackedRetrievalIndex) Refresh(ctx context.Context) error {
	if idx.Index == nil {
		return nil
	}
	chunks, err := idx.Store.ListIndexedChunks(ctx)
	if err != nil {
		return err
	}
	return idx.Index.Rebuild(ctx, chunks)
}
