package cmd

import (
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"time"

	"github.com/Sahil0114/infrasage1/internal/gitops"
	"github.com/Sahil0114/infrasage1/internal/monitor"
	"github.com/Sahil0114/infrasage1/internal/ui"
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the InfraSage browser dashboard",
	Long: `Starts the local InfraSage UI at http://localhost:3000.
It provides a streaming terminal, pipeline visualizer, scan dashboard,
Grafana embed, and drift remediation controls.`,
	RunE: runUI,
}

func init() {
	rootCmd.AddCommand(uiCmd)
}

func runUI(cmd *cobra.Command, args []string) error {
	uiPort := getEnvOrDefault("INFRASAGE_UI_PORT", "3000")
	grafanaURL := getEnvOrDefault("INFRASAGE_GRAFANA_URL", "http://localhost:3001")
	metricsPort := getEnvOrDefault("INFRASAGE_METRICS_PORT", "2112")

	monitor.StartServer(metricsPort)

	awsCreds, awsAvailable, err := gitops.DetectAWSCredentials()
	if err != nil {
		slog.Warn("aws detection failed", "err", err)
	}

	if awsAvailable {
		if err := gitops.SyncAWSSecrets(awsCreds); err != nil {
			slog.Warn("failed to sync aws secrets", "err", err)
			awsAvailable = false
		}
	}

	server := ui.NewServer(ui.Config{
		Addr:         ":" + uiPort,
		GrafanaURL:   grafanaURL,
		SystemPrompt: systemPrompt,
		AWSAvailable: awsAvailable,
		Mode:         "github",
	})

	uiURL := fmt.Sprintf("http://localhost:%s", uiPort)
	fmt.Printf("🚀 InfraSage UI starting at %s\n", uiURL)
	fmt.Println("   Press Ctrl+C to stop.")

	go func() {
		time.Sleep(200 * time.Millisecond)
		if err := openBrowser(uiURL); err != nil {
			fmt.Printf("⚠️  Unable to open browser automatically: %v\n", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("ui server stopped: %w", err)
	}
	return nil
}

func openBrowser(url string) error {
	var openCmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		openCmd = exec.Command("open", url)
	case "linux":
		openCmd = exec.Command("xdg-open", url)
	case "windows":
		openCmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS %q — open %s manually in your browser", runtime.GOOS, url)
	}
	return openCmd.Run()
}
