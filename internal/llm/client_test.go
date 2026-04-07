package llm

import (
	"testing"
	"time"
)

// TestIsHealthy_WhenOllamaRunning verifies that IsHealthy returns true
// when Ollama is running locally on port 11434.
//
// This test requires Ollama to be running natively on macOS:
//
//	ollama serve   (or brew services start ollama)
//
// Run with: go test ./internal/llm/... -v
func TestIsHealthy_WhenOllamaRunning(t *testing.T) {
	client := NewOllamaClient("http://localhost:11434", "qwen2.5-coder:3b")
	if !client.IsHealthy() {
		t.Skip("Ollama is not running — skipping live health check test.\n" +
			"Fix: run 'ollama serve' in a terminal, then re-run tests.")
	}
}

// TestNewOllamaClient verifies that the constructor sets fields correctly.
func TestNewOllamaClient_Fields(t *testing.T) {
	client := NewOllamaClient("http://localhost:11434", "qwen2.5-coder:3b")

	if client.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL: got %q, want %q", client.BaseURL, "http://localhost:11434")
	}
	if client.Model != "qwen2.5-coder:3b" {
		t.Errorf("Model: got %q, want %q", client.Model, "qwen2.5-coder:3b")
	}
	if client.Timeout != 120*time.Second {
		t.Errorf("Timeout: got %v, want %v", client.Timeout, 120*time.Second)
	}
}

// TestIsHealthy_WhenOllamaNotRunning verifies that IsHealthy returns false
// when the given URL points to nothing.
func TestIsHealthy_WhenOllamaNotRunning(t *testing.T) {
	// Port 19999 should have nothing listening on a typical dev machine.
	client := NewOllamaClient("http://localhost:19999", "qwen2.5-coder:3b")
	if client.IsHealthy() {
		t.Error("IsHealthy: expected false for a port with no listener, got true")
	}
}

// TestGenerate_LiveOllama performs a real generation call against a running Ollama
// instance. It is skipped automatically if Ollama is not available.
//
// This test validates:
//   - The Generate method returns a non-empty string
//   - The response contains recognisable Terraform keywords
func TestGenerate_LiveOllama(t *testing.T) {
	client := NewOllamaClient("http://localhost:11434", "qwen2.5-coder:3b")

	if !client.IsHealthy() {
		t.Skip("Ollama is not running — skipping live generation test.\n" +
			"Fix: run 'ollama serve' in a terminal, then re-run tests.")
	}

	// Use a very short prompt to keep the test fast.
	// The system prompt guides the model to output HCL.
	const system = "You are a Terraform expert. Output ONLY raw HCL code. No explanations."
	const prompt = "Create a minimal terraform block with aws provider ~> 5.0 and required_version >= 1.6."

	response, err := client.Generate(system, prompt)
	if err != nil {
		t.Fatalf("Generate returned unexpected error: %v", err)
	}

	if response == "" {
		t.Fatal("Generate returned an empty response — model may not be loaded.")
	}

	t.Logf("Generate response (first 200 chars): %.200s", response)
}
