package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"office-assistant/internal/config"
	"office-assistant/internal/server"
	"office-assistant/internal/storage"
)

func main() {
	configPath := flag.String("config", config.PathFromEnv(), "path to JSON config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))

	store, err := storage.Open(cfg.Storage.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.EnsureSchema(context.Background()); err != nil {
		logger.Error("ensure schema", "error", err)
		os.Exit(1)
	}

	handler := server.New(server.Options{
		Store:         store,
		Logger:        logger,
		JWTSecret:     cfg.Security.JWTSecret,
		UploadDir:     cfg.Storage.UploadDir,
		MarkItDownURL: cfg.Services.MarkItDownURL,
	})

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("backend listening", "addr", cfg.HTTP.Addr, "database", cfg.Storage.DatabasePath, "config", *configPath)
		errs <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-errs:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
}
