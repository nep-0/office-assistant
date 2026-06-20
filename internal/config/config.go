package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	HTTP          HTTPConfig          `json:"http"`
	Storage       StorageConfig       `json:"storage"`
	Services      ServicesConfig      `json:"services"`
	Security      SecurityConfig      `json:"security"`
	Observability ObservabilityConfig `json:"observability"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type StorageConfig struct {
	DatabasePath string `json:"database_path"`
	UploadDir    string `json:"upload_dir"`
}

type ServicesConfig struct {
	MarkItDownURL string `json:"markitdown_url"`
}

type SecurityConfig struct {
	JWTSecret string `json:"jwt_secret"`
}

type ObservabilityConfig struct {
	LogLevel string `json:"log_level"`
}

func Default() Config {
	return Config{
		HTTP: HTTPConfig{
			Addr: ":8080",
		},
		Storage: StorageConfig{
			DatabasePath: "data/office-assistant.db",
			UploadDir:    "data/uploads",
		},
		Services: ServicesConfig{
			MarkItDownURL: "",
		},
		Security: SecurityConfig{
			JWTSecret: "office-assistant-dev-secret",
		},
		Observability: ObservabilityConfig{
			LogLevel: "info",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	defaults := Default()
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		c.HTTP.Addr = defaults.HTTP.Addr
	}
	if strings.TrimSpace(c.Storage.DatabasePath) == "" {
		c.Storage.DatabasePath = defaults.Storage.DatabasePath
	}
	if strings.TrimSpace(c.Storage.UploadDir) == "" {
		c.Storage.UploadDir = defaults.Storage.UploadDir
	}
	if strings.TrimSpace(c.Security.JWTSecret) == "" {
		c.Security.JWTSecret = defaults.Security.JWTSecret
	}
	if strings.TrimSpace(c.Observability.LogLevel) == "" {
		c.Observability.LogLevel = defaults.Observability.LogLevel
	}
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.Observability.LogLevel)) {
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

func PathFromEnv() string {
	return strings.TrimSpace(os.Getenv("OA_CONFIG"))
}
