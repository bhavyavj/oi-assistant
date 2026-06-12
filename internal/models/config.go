package models

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port         string        `yaml:"port"`
		ReadTimeout  time.Duration `yaml:"read_timeout"`
		WriteTimeout time.Duration `yaml:"write_timeout"`
	} `yaml:"server"`

	Redis struct {
		Addr     string        `yaml:"addr"`
		Password string        `yaml:"password"`
		DB       int           `yaml:"db"`
		TTL      time.Duration `yaml:"ttl"`
	} `yaml:"redis"`

	LLM struct {
		Provider       string        `yaml:"provider"`
		OpenAIAPIKey   string        `yaml:"-"` // env-only: OPENAI_API_KEY
		OpenAIModel    string        `yaml:"openai_model"`
		OllamaURL      string        `yaml:"ollama_url"`
		OllamaModel    string        `yaml:"ollama_model"`
		Timeout        time.Duration `yaml:"timeout"`
		MaxConcurrency int           `yaml:"max_concurrency"`
	} `yaml:"llm"`

	Analyzer struct {
		OIChangeThreshold float64 `yaml:"oi_change_threshold"`
	} `yaml:"analyzer"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}

	// Env overrides
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.LLM.OpenAIAPIKey = key
	}
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		cfg.LLM.Provider = provider
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		cfg.Redis.Addr = addr
	}

	return &cfg, nil
}
