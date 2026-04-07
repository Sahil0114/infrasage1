package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// OllamaClient is an HTTP client for a locally running Ollama instance.
type OllamaClient struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewOllamaClient creates a new OllamaClient with a 120-second generation timeout.
func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 120 * time.Second,
	}
}

// Generate sends a prompt to Ollama and returns the raw model response string.
// It uses a low temperature (0.1) for deterministic, code-focused output.
func (c *OllamaClient) Generate(systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":  c.Model,
		"prompt": userPrompt,
		"system": systemPrompt,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.1,
			"num_ctx":     4096,
			"top_p":       0.9,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm Generate: marshaling request body: %w", err)
	}

	slog.Debug("sending generate request to ollama",
		"url", c.BaseURL+"/api/generate",
		"model", c.Model,
		"prompt_len", len(userPrompt),
	)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llm Generate: creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("llm Generate: Ollama did not respond within %s — model may be loading, try again", c.Timeout)
		}
		return "", fmt.Errorf("llm Generate: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm Generate: Ollama returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("llm Generate: decoding Ollama response: %w", err)
	}

	if result.Error != "" {
		return "", fmt.Errorf("llm Generate: Ollama error: %s", result.Error)
	}

	if result.Response == "" {
		return "", fmt.Errorf("llm Generate: Ollama returned an empty response — model '%s' may not be pulled yet.\nFix: run 'infrasage model pull'", c.Model)
	}

	slog.Debug("received response from ollama", "response_len", len(result.Response), "done", result.Done)

	return result.Response, nil
}

// IsHealthy checks whether the Ollama server is reachable.
// Uses a short 3-second timeout — this is a liveness check, not a generation call.
func (c *OllamaClient) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		slog.Debug("ollama health check: failed to build request", "err", err)
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("ollama health check: request failed", "err", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
