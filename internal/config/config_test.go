package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWithoutPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("unexpected default addr: %q", cfg.HTTP.Addr)
	}
	if cfg.Storage.DatabasePath == "" {
		t.Fatal("expected default database path")
	}
	if cfg.Security.JWTSecret == "" {
		t.Fatal("expected default jwt secret")
	}
}

func TestLoadJSONConfigWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
		"http": {"addr": ":9090"},
		"storage": {"database_path": "custom.db"},
		"observability": {"log_level": "debug"}
	}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTP.Addr != ":9090" {
		t.Fatalf("unexpected addr: %q", cfg.HTTP.Addr)
	}
	if cfg.Storage.DatabasePath != "custom.db" {
		t.Fatalf("unexpected database path: %q", cfg.Storage.DatabasePath)
	}
	if cfg.Storage.UploadDir != "data/uploads" {
		t.Fatalf("expected default upload dir, got %q", cfg.Storage.UploadDir)
	}
	if cfg.LogLevel().String() != "DEBUG" {
		t.Fatalf("expected debug log level, got %s", cfg.LogLevel())
	}
}
