package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// modelCmd is the parent command for model management operations.
var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage the Ollama model used for HCL generation",
	Long: `Manage the local Ollama model.

The model name is read from INFRASAGE_OLLAMA_MODEL (default: qwen2.5-coder:3b).
Ollama must be running natively on macOS — not inside Docker.`,
}

var modelPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull (download) the configured Ollama model",
	Long: `Download the model from the Ollama registry to ~/.ollama/models/.

This is a one-time operation (~2 GB download). Re-running it updates to the latest version.
The model is managed entirely by Ollama — no model files are stored in this repository.`,
	RunE: runModelPull,
}

var modelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "List all locally available Ollama models",
	Long:  `Runs 'ollama list' to show downloaded models, their sizes, and last-modified time.`,
	RunE:  runModelStatus,
}

func init() {
	modelCmd.AddCommand(modelPullCmd)
	modelCmd.AddCommand(modelStatusCmd)
	rootCmd.AddCommand(modelCmd)
}

func runModelPull(cmd *cobra.Command, args []string) error {
	model := getEnvOrDefault("INFRASAGE_OLLAMA_MODEL", "qwen2.5-coder:3b")

	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama not found in PATH.\nFix: install Ollama from https://ollama.com or run 'brew install ollama'")
	}

	fmt.Printf("⬇  Pulling model: %s\n", model)
	fmt.Println("   This may take several minutes on first run (~2 GB download).")
	fmt.Println()

	ollamaCmd := exec.Command("ollama", "pull", model)
	ollamaCmd.Stdout = os.Stdout
	ollamaCmd.Stderr = os.Stderr

	if err := ollamaCmd.Run(); err != nil {
		return fmt.Errorf("ollama pull %s failed: %w\nFix: ensure Ollama is running ('ollama serve') and you have internet access.", model, err)
	}

	fmt.Printf("\n✅ Model '%s' is ready.\n", model)
	fmt.Printf("   Run 'infrasage ask \"...\"' to generate Terraform HCL.\n")
	return nil
}

func runModelStatus(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama not found in PATH.\nFix: install Ollama from https://ollama.com or run 'brew install ollama'")
	}

	configured := getEnvOrDefault("INFRASAGE_OLLAMA_MODEL", "qwen2.5-coder:3b")
	fmt.Printf("🤖 Configured model: %s\n\n", configured)
	fmt.Println("Downloaded models (ollama list):")

	ollamaCmd := exec.Command("ollama", "list")
	ollamaCmd.Stdout = os.Stdout
	ollamaCmd.Stderr = os.Stderr

	if err := ollamaCmd.Run(); err != nil {
		return fmt.Errorf("ollama list failed: %w\nFix: ensure Ollama is running ('ollama serve' or 'brew services start ollama').", err)
	}

	// Also show currently loaded models if any.
	fmt.Println("\nCurrently loaded models (ollama ps):")
	psCmd := exec.Command("ollama", "ps")
	psCmd.Stdout = os.Stdout
	psCmd.Stderr = os.Stderr
	// ollama ps may fail on older Ollama versions — ignore the error gracefully.
	if err := psCmd.Run(); err != nil {
		fmt.Println("  (ollama ps not available on this Ollama version)")
	}

	return nil
}
