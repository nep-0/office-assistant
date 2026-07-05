package search

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"office-assistant/backend/domain"

	chromem "github.com/philippgille/chromem-go"
)

const vectorCollectionName = "document_chunks"

type VectorIndex struct {
	mu          sync.RWMutex
	db          *chromem.DB
	collection  *chromem.Collection
	embed       chromem.EmbeddingFunc
	persistRoot string
	count       int
}

type VectorSearchResult struct {
	ChunkID    int64
	DocumentID int64
	Content    string
	Similarity float32
}

func NewVectorIndex(embed chromem.EmbeddingFunc, persistRoot string) (*VectorIndex, error) {
	db, err := chromem.NewPersistentDB(filepath.Join(persistRoot, "chromem-go"), false)
	if err != nil {
		return nil, err
	}
	collection, err := db.GetOrCreateCollection(vectorCollectionName, nil, embed)
	if err != nil {
		return nil, err
	}
	return &VectorIndex{db: db, collection: collection, embed: embed, persistRoot: persistRoot, count: collection.Count()}, nil
}

func NewInMemoryVectorIndex(embed chromem.EmbeddingFunc) (*VectorIndex, error) {
	db := chromem.NewDB()
	collection, err := db.GetOrCreateCollection(vectorCollectionName, nil, embed)
	if err != nil {
		return nil, err
	}
	return &VectorIndex{db: db, collection: collection, embed: embed, count: collection.Count()}, nil
}

func (idx *VectorIndex) Rebuild(ctx context.Context, chunks []domain.DocumentChunk) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	var db *chromem.DB
	var err error
	if idx.persistRoot == "" {
		db = chromem.NewDB()
	} else {
		db, err = chromem.NewPersistentDB(filepath.Join(idx.persistRoot, "chromem-go"), false)
		if err != nil {
			return err
		}
		if err := db.DeleteCollection(vectorCollectionName); err != nil {
			return err
		}
	}
	collection, err := db.GetOrCreateCollection(vectorCollectionName, nil, idx.embed)
	if err != nil {
		return err
	}
	docs := make([]chromem.Document, 0, len(chunks))
	for _, chunk := range chunks {
		docs = append(docs, chromem.Document{
			ID:      fmt.Sprintf("%d", chunk.ID),
			Content: chunk.Content,
			Metadata: map[string]string{
				"chunk_id":    fmt.Sprintf("%d", chunk.ID),
				"document_id": fmt.Sprintf("%d", chunk.DocumentID),
				"version_id":  fmt.Sprintf("%d", chunk.DocumentVersionID),
				"chunk_no":    fmt.Sprintf("%d", chunk.ChunkNo),
			},
		})
	}
	if len(docs) > 0 {
		if err := collection.AddDocuments(ctx, docs, 1); err != nil {
			return err
		}
	}
	idx.db = db
	idx.collection = collection
	idx.count = len(docs)
	return nil
}

func (idx *VectorIndex) Preflight(ctx context.Context, chunks []domain.DocumentChunk) error {
	probe, err := NewInMemoryVectorIndex(idx.embed)
	if err != nil {
		return err
	}
	probeChunks := make([]domain.DocumentChunk, len(chunks))
	copy(probeChunks, chunks)
	for i := range probeChunks {
		probeChunks[i].ID = int64(i + 1)
		probeChunks[i].DocumentID = 1
		probeChunks[i].DocumentVersionID = 1
	}
	return probe.Rebuild(ctx, probeChunks)
}

func (idx *VectorIndex) Search(ctx context.Context, query string, limit int) ([]VectorSearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.collection == nil || idx.count == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > idx.count {
		limit = idx.count
	}
	results, err := idx.collection.Query(ctx, query, limit, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]VectorSearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, VectorSearchResult{
			ChunkID:    parseMetadataInt(result.Metadata["chunk_id"]),
			DocumentID: parseMetadataInt(result.Metadata["document_id"]),
			Content:    result.Content,
			Similarity: result.Similarity,
		})
	}
	return out, nil
}

func parseMetadataInt(value string) int64 {
	var result int64
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		result = result*10 + int64(r-'0')
	}
	return result
}
