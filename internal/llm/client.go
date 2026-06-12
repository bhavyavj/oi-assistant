package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is the provider-agnostic LLM interface.
type Client interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// --- OpenAI ---

type openAIClient struct {
	apiKey  string
	model   string
	timeout time.Duration
	http    *http.Client
}

func NewOpenAI(apiKey, model string, timeout time.Duration) Client {
	return &openAIClient{
		apiKey:  apiKey,
		model:   model,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *openAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  512,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai status %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// --- Ollama ---

type ollamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllama(baseURL, model string, timeout time.Duration) Client {
	return &ollamaClient{
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *ollamaClient) Complete(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  c.model,
		"prompt": prompt,
		"stream": false,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return result.Response, nil
}

// NewFromConfig returns the right client based on provider string.
func NewFromConfig(provider, openaiKey, openaiModel, ollamaURL, ollamaModel string, timeout time.Duration) (Client, error) {
	switch provider {
	case "openai":
		if openaiKey == "" {
			return nil, fmt.Errorf("openai provider selected but OPENAI_API_KEY is empty")
		}
		return NewOpenAI(openaiKey, openaiModel, timeout), nil
	case "ollama":
		return NewOllama(ollamaURL, ollamaModel, timeout), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q (supported: openai, ollama)", provider)
	}
}
