package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bhavyavj/oi-assistant/internal/api"
	"github.com/bhavyavj/oi-assistant/internal/api/handlers"
	"github.com/bhavyavj/oi-assistant/internal/core"
	"github.com/bhavyavj/oi-assistant/internal/llm"
	"github.com/bhavyavj/oi-assistant/internal/models"
	"github.com/bhavyavj/oi-assistant/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := models.LoadConfig(cfgPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Storage (Redis with In-Memory fallback)
	var store storage.Store
	store, err = storage.NewRedisStore(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.TTL)
	if err != nil {
		log.Warn("failed to connect to redis, falling back to in-memory storage", "error", err)
		store = storage.NewInMemoryStore(cfg.Redis.TTL)
	} else {
		log.Info("redis connected", "addr", cfg.Redis.Addr)
	}
	defer store.Close()

	// LLM client
	llmClient, err := llm.NewFromConfig(
		cfg.LLM.Provider,
		cfg.LLM.OpenAIAPIKey,
		cfg.LLM.OpenAIModel,
		cfg.LLM.OllamaURL,
		cfg.LLM.OllamaModel,
		cfg.LLM.Timeout,
	)
	if err != nil {
		log.Error("failed to init LLM client", "error", err)
		os.Exit(1)
	}
	log.Info("LLM client ready", "provider", cfg.LLM.Provider)

	// Analyzer
	analyzer := core.NewAnalyzer(llmClient, cfg.Analyzer.OIChangeThreshold, cfg.LLM.MaxConcurrency, log)

	// HTTP server
	h := handlers.New(store, analyzer, log)
	router := api.NewRouter(h)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start in background
	serverErr := make(chan error, 1)
	go func() {
		log.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Error("server error", "error", err)
	case <-quit:
		log.Info("shutting down...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	log.Info("server stopped")
}
