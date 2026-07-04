package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

const vectorCollectionName = "document_chunks"

type vectorIndex struct {
	mu          sync.RWMutex
	db          *chromem.DB
	collection  *chromem.Collection
	embed       chromem.EmbeddingFunc
	persistRoot string
	count       int
}

type vectorSearchResult struct {
	ChunkID    int64
	DocumentID int64
	Content    string
	Similarity float32
}

func newVectorIndex(embed chromem.EmbeddingFunc, persistRoot string) (*vectorIndex, error) {
	db, err := chromem.NewPersistentDB(filepath.Join(persistRoot, "chromem-go"), false)
	if err != nil {
		return nil, err
	}
	collection, err := db.GetOrCreateCollection(vectorCollectionName, nil, embed)
	if err != nil {
		return nil, err
	}
	return &vectorIndex{db: db, collection: collection, embed: embed, persistRoot: persistRoot, count: collection.Count()}, nil
}

func newInMemoryVectorIndex(embed chromem.EmbeddingFunc) (*vectorIndex, error) {
	db := chromem.NewDB()
	collection, err := db.GetOrCreateCollection(vectorCollectionName, nil, embed)
	if err != nil {
		return nil, err
	}
	return &vectorIndex{db: db, collection: collection, embed: embed, count: collection.Count()}, nil
}

func (idx *vectorIndex) rebuild(ctx context.Context, chunks []documentChunk) error {
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

func (idx *vectorIndex) search(ctx context.Context, query string, limit int) ([]vectorSearchResult, error) {
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
	out := make([]vectorSearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, vectorSearchResult{
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

func (a *app) embeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		setting, err := a.store.findProviderSetting(ctx, providerPurposeEmbedding)
		if err != nil {
			return nil, err
		}
		return a.openAICompatibleEmbedding(ctx, setting, text)
	}
}

func (a *app) openAICompatibleEmbedding(ctx context.Context, setting providerSetting, text string) ([]float32, error) {
	if strings.Contains(setting.BaseURL, "/fake-openai") {
		return deterministicEmbedding(text, 64), nil
	}
	started := time.Now()
	log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_started", correlationID(ctx), setting.Model)
	endpoint, err := joinProviderPath(setting.BaseURL, "embeddings")
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model": setting.Model,
		"input": text,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if setting.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+setting.APIKey)
	}
	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_error error=%q", correlationID(ctx), setting.Model, err.Error())
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_error status=%s", correlationID(ctx), setting.Model, res.Status)
		return nil, fmt.Errorf("embedding provider returned %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 20<<20)).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, errors.New("embedding provider returned no embedding")
	}
	log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_completed duration_ms=%d", correlationID(ctx), setting.Model, time.Since(started).Milliseconds())
	return decoded.Data[0].Embedding, nil
}

func joinProviderPath(base, suffix string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Join(parsed.Path, suffix)
	return parsed.String(), nil
}

func deterministicEmbedding(text string, dims int) []float32 {
	if dims <= 0 {
		dims = 64
	}
	vector := make([]float32, dims)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		sum := sha256.Sum256([]byte(word))
		index := int(binary.BigEndian.Uint64(sum[:8]) % uint64(dims))
		vector[index] += 1
	}
	var magnitude float64
	for _, value := range vector {
		magnitude += float64(value * value)
	}
	if magnitude == 0 {
		vector[0] = 1
		return vector
	}
	scale := float32(math.Sqrt(magnitude))
	for i := range vector {
		vector[i] /= scale
	}
	return vector
}
