package ingestion

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"office-assistant/backend/domain"
)

func TestExtractDocumentIncludesServiceErrorMessage(t *testing.T) {
	root := t.TempDir()
	doc := domain.DocumentRecord{StorageKey: "original", OriginalFilename: "scan.png"}
	if err := os.WriteFile(filepath.Join(root, doc.StorageKey), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnprocessableEntity,
			Status:     "422 Unprocessable Entity",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":"ocr_timeout","message":"OCR service timed out"}`)),
			Request:    request,
		}, nil
	})}

	_, err := ExtractDocument(context.Background(), client, "http://document.test", root, doc)
	if err == nil || !strings.Contains(err.Error(), "OCR service timed out") {
		t.Fatalf("error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
