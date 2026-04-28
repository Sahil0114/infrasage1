// HUMAN SETUP REQUIRED (one-time, before running this binary):
// 1. brew install ollama
// 2. brew install terraform
// 3. ollama pull qwen2.5-coder:3b
// 4. ollama serve   (keep this terminal open, or run: brew services start ollama)

package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// getEnvOrDefaultStack returns the value of key from the environment,
// or defaultVal if it is empty.
// (Each cmd file declares its own helper to stay self-contained.)
func getEnvOrDefaultStack(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// composeFile returns the path to the docker-compose.yml, configurable via env.
func composeFile() string {
	return getEnvOrDefaultStack("INFRASAGE_COMPOSE_FILE", "./deploy/docker-compose.yml")
}

// stackCmd is the parent command: `infrasage stack`
var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage the InfraSage Docker stack (scanner containers + monitoring)",
	Long: `stack manages the Docker Compose stack that runs the three security
scanner containers (checkov, tfsec, terrascan) plus Prometheus and Grafana.

Ollama runs natively on macOS — it is NOT part of the Docker stack.
stack up checks whether Ollama is running and starts it if needed.`,
}

var stackUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start scanner containers, Prometheus, Grafana, and verify Ollama",
	RunE:  runStackUp,
}

var stackDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all InfraSage Docker containers",
	RunE:  runStackDown,
}

var stackStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of all InfraSage containers",
	RunE:  runStackStatus,
}

func init() {
	stackCmd.AddCommand(stackUpCmd)
	stackCmd.AddCommand(stackDownCmd)
	stackCmd.AddCommand(stackStatusCmd)
	rootCmd.AddCommand(stackCmd)
}

// runStackUp starts the full InfraSage stack:
//  1. Creates the shared scan directory.
//  2. Verifies / starts Ollama (native macOS process).
//  3. Runs docker compose up -d.
func runStackUp(cmd *cobra.Command, args []string) error {
	// Step 1 — ensure the shared scan volume directory exists on the host.
	scanDir := getEnvOrDefaultStack("INFRASAGE_SCAN_DIR", "/tmp/infrasage-scans")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		return fmt.Errorf("stack up: creating scan directory %s: %w\nFix: check directory permissions or set INFRASAGE_SCAN_DIR to a writable path.", scanDir, err)
	}
	slog.Debug("scan directory ready", "path", scanDir)
	fmt.Printf("📁 Scan directory: %s\n", scanDir)

	// Step 2 — check Ollama availability.
	if _, err := exec.LookPath("ollama"); err != nil {
		fmt.Println("⚠️  ollama binary not found in PATH.")
		fmt.Println("   Fix: brew install ollama  (then run: ollama pull qwen2.5-coder:3b)")
		// Do not exit — user may have Ollama installed elsewhere.
	}

	if isOllamaRunning() {
		fmt.Println("✅ Ollama already running")
	} else {
		fmt.Println("🤖 Ollama not detected — attempting to start...")
		ollamaCmd := exec.Command("ollama", "serve")
		if err := ollamaCmd.Start(); err != nil {
			fmt.Printf("⚠️  Could not start Ollama automatically: %v\n", err)
			fmt.Println("   Fix: open a new terminal and run: ollama serve")
		} else {
			fmt.Printf("🤖 Ollama started (PID: %d)\n", ollamaCmd.Process.Pid)
			// Detach — we don't wait for it. It runs as a background daemon.
			go func() { _ = ollamaCmd.Wait() }()
		}
	}

	// Step 3 — docker compose up.
	fmt.Println("\n🐳 Starting Docker containers...")
	cf := composeFile()

	dcUp := exec.Command("docker", "compose", "-f", cf, "up", "-d")
	dcUp.Stdout = os.Stdout
	dcUp.Stderr = os.Stderr

	if err := dcUp.Run(); err != nil {
		return fmt.Errorf("stack up: docker compose up failed: %w\nFix: ensure Docker Desktop is running and the compose file exists at %s", err, cf)
	}

	fmt.Println()
	fmt.Println("✅ InfraSage stack is up!")
	fmt.Println("   Prometheus: http://localhost:9091")
	fmt.Println("   Grafana:    http://localhost:3001  (admin / infrasage)")
	fmt.Println("   Ollama:     http://localhost:11434")
	fmt.Println()
	fmt.Println("   Run 'infrasage ask \"create an S3 bucket\"' to generate Terraform HCL.")
	return nil
}

// runStackDown stops all InfraSage containers.
func runStackDown(cmd *cobra.Command, args []string) error {
	fmt.Println("🛑 Stopping InfraSage stack...")
	cf := composeFile()

	dcDown := exec.Command("docker", "compose", "-f", cf, "down")
	dcDown.Stdout = os.Stdout
	dcDown.Stderr = os.Stderr

	if err := dcDown.Run(); err != nil {
		return fmt.Errorf("stack down: docker compose down failed: %w\nFix: ensure Docker Desktop is running.", err)
	}

	fmt.Println("✅ InfraSage stack stopped.")
	return nil
}

// runStackStatus shows the current status of all InfraSage containers.
func runStackStatus(cmd *cobra.Command, args []string) error {
	cf := composeFile()

	dcPs := exec.Command("docker", "compose", "-f", cf, "ps")
	dcPs.Stdout = os.Stdout
	dcPs.Stderr = os.Stderr

	if err := dcPs.Run(); err != nil {
		return fmt.Errorf("stack status: docker compose ps failed: %w\nFix: ensure Docker Desktop is running.", err)
	}
	return nil
}

// isOllamaRunning does a fast health check against the Ollama API.
// Returns true only if Ollama responds with HTTP 200 within 2 seconds.
func isOllamaRunning() bool {
	checkCmd := exec.Command("curl", "-sf", "--max-time", "2",
		"http://localhost:11434/api/tags")
	return checkCmd.Run() == nil
}
