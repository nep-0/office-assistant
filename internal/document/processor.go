package document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Processor interface {
	Process(ctx context.Context, input ProcessInput) ([]Chunk, error)
}

type ProcessInput struct {
	DocumentID      string
	KnowledgeBaseID string
	FilePath        string
	OriginalName    string
	ContentType     string
}

type Chunk struct {
	ID               string         `json:"chunk_id"`
	DocumentID       string         `json:"document_id"`
	KnowledgeBaseID  string         `json:"knowledge_base_id"`
	Content          string         `json:"content"`
	SourceFileName   string         `json:"source_file_name"`
	PageNumber       *int           `json:"page_number"`
	ChunkIndex       int            `json:"chunk_index"`
	ContentType      string         `json:"content_type"`
	TokenOrCharCount int            `json:"token_or_char_count"`
	Metadata         map[string]any `json:"metadata"`
}

type HTTPProcessor struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (p HTTPProcessor) Process(ctx context.Context, input ProcessInput) ([]Chunk, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, fmt.Errorf("markitdown processor base URL is not configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("document_id", input.DocumentID); err != nil {
		return nil, fmt.Errorf("write document_id field: %w", err)
	}
	if err := writer.WriteField("knowledge_base_id", input.KnowledgeBaseID); err != nil {
		return nil, fmt.Errorf("write knowledge_base_id field: %w", err)
	}
	if err := writer.WriteField("content_type", input.ContentType); err != nil {
		return nil, fmt.Errorf("write content_type field: %w", err)
	}

	file, err := os.Open(input.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(input.OriginalName))
	if err != nil {
		return nil, fmt.Errorf("create file part: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy file part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/process", &body)
	if err != nil {
		return nil, fmt.Errorf("create processor request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("send processor request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("processor returned %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}

	var output struct {
		Chunks []Chunk `json:"chunks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("decode processor response: %w", err)
	}
	if len(output.Chunks) == 0 {
		return nil, fmt.Errorf("processor returned no chunks")
	}
	return output.Chunks, nil
}

func (p HTTPProcessor) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}
