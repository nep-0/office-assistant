package main

import (
	"context"
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
	cfg := config.FromEnv()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	store, err := storage.Open(cfg.DatabasePath)
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
		JWTSecret:     cfg.JWTSecret,
		UploadDir:     cfg.UploadDir,
		MarkItDownURL: cfg.MarkItDownURL,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("backend listening", "addr", cfg.HTTPAddr, "database", cfg.DatabasePath)
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
