package main

import (
	"context"
	"fmt"
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
	"github.com/bhavyavj/oi-assistant/pkg/logger"
)

func main() {
	log := logger.New()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := models.LoadConfig(cfgPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Redis
	store, err := storage.NewRedisStore(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.TTL)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	log.Info("redis connected", "addr", cfg.Redis.Addr)

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
	go func() {
		log.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	log.Info("server stopped")
}
