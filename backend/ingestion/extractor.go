package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"office-assistant/backend/domain"
)

type ExtractionPackage struct {
	SchemaVersion string           `json:"schema_version"`
	Markdown      string           `json:"markdown"`
	Metadata      map[string]any   `json:"metadata"`
	Warnings      []string         `json:"warnings"`
	OCR           map[string]any   `json:"ocr"`
	SourceAnchors []map[string]any `json:"source_anchors"`
}

func ExtractDocument(ctx context.Context, client *http.Client, documentURL, storageRoot string, doc domain.DocumentRecord) (ExtractionPackage, error) {
	file, err := os.Open(filepath.Join(storageRoot, doc.StorageKey))
	if err != nil {
		return ExtractionPackage{}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", doc.OriginalFilename)
	if err != nil {
		return ExtractionPackage{}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return ExtractionPackage{}, err
	}
	if err := writer.Close(); err != nil {
		return ExtractionPackage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, documentURL+"/extract", &body)
	if err != nil {
		return ExtractionPackage{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return ExtractionPackage{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ExtractionPackage{}, UnexpectedStatus(res.Status)
	}

	var pkg ExtractionPackage
	if err := json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(&pkg); err != nil {
		return ExtractionPackage{}, err
	}
	return pkg, nil
}

func WriteExtractedMarkdown(storageRoot string, doc domain.DocumentRecord, markdown string) (string, error) {
	markdownKey := filepath.Join(filepath.Dir(doc.StorageKey), "extracted.md")
	fullPath := filepath.Join(storageRoot, markdownKey)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	return markdownKey, os.WriteFile(fullPath, []byte(markdown), 0o644)
}

type UnexpectedStatus string

func (err UnexpectedStatus) Error() string {
	return "document service returned " + string(err)
}
