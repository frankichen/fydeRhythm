package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/frankichen/fydeRhythm/sync-server/internal/api"
	"github.com/frankichen/fydeRhythm/sync-server/internal/config"
	"github.com/frankichen/fydeRhythm/sync-server/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	router, err := api.New(api.Options{
		Store: db, APIToken: cfg.APIToken, CORSOrigins: cfg.CORSOrigins,
		MaxBodyBytes: cfg.MaxBodyBytes, Logger: logger,
	})
	if err != nil {
		logger.Error("create HTTP router", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: cfg.ListenAddr, Handler: router,
		ReadTimeout: cfg.ReadTimeout, ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout,
	}

	go func() {
		logger.Info("server_started", "address", cfg.ListenAddr, "database", cfg.DatabasePath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server_stopped")
}
