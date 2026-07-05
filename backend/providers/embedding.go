package providers

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
	"strings"
	"time"

	"office-assistant/backend/domain"
)

func OpenAICompatibleEmbedding(ctx context.Context, client *http.Client, setting domain.ProviderSetting, text, correlationID string) ([]float32, error) {
	if strings.Contains(setting.BaseURL, "/fake-openai") {
		return DeterministicEmbedding(text, 64), nil
	}
	started := time.Now()
	log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_started", correlationID, setting.Model)
	endpoint, err := JoinProviderPath(setting.BaseURL, "embeddings")
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
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_error error=%q", correlationID, setting.Model, err.Error())
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_error status=%s", correlationID, setting.Model, res.Status)
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
	log.Printf("correlation_id=%s provider=embedding model=%s event=provider_call_completed duration_ms=%d", correlationID, setting.Model, time.Since(started).Milliseconds())
	return decoded.Data[0].Embedding, nil
}

func JoinProviderPath(base, suffix string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Join(parsed.Path, suffix)
	return parsed.String(), nil
}

func DeterministicEmbedding(text string, dims int) []float32 {
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
