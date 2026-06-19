package config

import (
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr      string
	DatabasePath  string
	UploadDir     string
	MarkItDownURL string
	JWTSecret     string
	LogLevel      slog.Level
}

func FromEnv() Config {
	return Config{
		HTTPAddr:      env("OA_HTTP_ADDR", ":8080"),
		DatabasePath:  env("OA_DATABASE_PATH", "data/office-assistant.db"),
		UploadDir:     env("OA_UPLOAD_DIR", "data/uploads"),
		MarkItDownURL: env("OA_MARKITDOWN_URL", ""),
		JWTSecret:     env("OA_JWT_SECRET", "office-assistant-dev-secret"),
		LogLevel:      logLevel(env("OA_LOG_LEVEL", "info")),
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func logLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
