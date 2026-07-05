package providers

import (
	"net/url"
	"strings"

	"office-assistant/backend/domain"
)

const (
	PurposeChat      = "chat"
	PurposeEmbedding = "embedding"
)

type ValidationError struct {
	Code    string
	Message string
}

func (err ValidationError) Error() string {
	return err.Code
}

func ValidateSetting(setting domain.ProviderSetting) *ValidationError {
	if !ValidPurpose(setting.Purpose) {
		return &ValidationError{Code: "provider_not_found", Message: "provider purpose not found"}
	}
	if strings.TrimSpace(setting.BaseURL) == "" {
		return &ValidationError{Code: "provider_base_url_required", Message: "provider base URL is required"}
	}
	parsed, err := url.Parse(setting.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return &ValidationError{Code: "provider_base_url_invalid", Message: "provider base URL must be an absolute HTTP URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &ValidationError{Code: "provider_base_url_invalid", Message: "provider base URL must use HTTP or HTTPS"}
	}
	if strings.TrimSpace(setting.Model) == "" {
		return &ValidationError{Code: "provider_model_required", Message: "provider model is required"}
	}
	return nil
}

func ValidPurpose(purpose string) bool {
	return purpose == PurposeChat || purpose == PurposeEmbedding
}

func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}
