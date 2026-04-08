package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Interact with the InfraSage monitoring stack",
	Long: `Interact with Prometheus and Grafana.

Commands:
  open     Open Grafana dashboard in your default browser
	metrics  Print raw Prometheus metrics to stdout
	serve    Keep a persistent /metrics endpoint running`,
}

var monitorOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open Grafana in the default browser",
	RunE:  runMonitorOpen,
}

var monitorMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Print Prometheus metrics to stdout",
	RunE:  runMonitorMetrics,
}

var monitorServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a persistent Prometheus metrics endpoint",
	Long: `Runs a long-lived HTTP server exposing /metrics.

Use this when Grafana shows no data because short-lived CLI commands exit
before Prometheus can scrape metrics consistently.`,
	RunE: runMonitorServe,
}

func init() {
	monitorCmd.AddCommand(monitorOpenCmd)
	monitorCmd.AddCommand(monitorMetricsCmd)
	monitorCmd.AddCommand(monitorServeCmd)
	rootCmd.AddCommand(monitorCmd)
}

func runMonitorOpen(_ *cobra.Command, _ []string) error {
	grafanaURL := getEnvOrDefault("INFRASAGE_GRAFANA_URL", "http://localhost:3000")

	fmt.Printf("🌐 Opening Grafana at %s\n", grafanaURL)
	fmt.Printf("   Default credentials: admin / infrasage\n\n")

	var openCmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		openCmd = exec.Command("open", grafanaURL)
	case "linux":
		openCmd = exec.Command("xdg-open", grafanaURL)
	case "windows":
		openCmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", grafanaURL)
	default:
		return fmt.Errorf("unsupported OS %q — open %s manually in your browser", runtime.GOOS, grafanaURL)
	}

	if err := openCmd.Run(); err != nil {
		return fmt.Errorf("failed to open browser: %w\nFix: open %s manually in your browser.", err, grafanaURL)
	}

	fmt.Println("✅ Browser opened.")
	fmt.Printf("   If Grafana shows no data, ensure:\n")
	fmt.Printf("   1. 'infrasage stack up' has been run\n")
	fmt.Printf("   2. The binary has been run at least once (to emit metrics)\n")
	fmt.Printf("   3. Prometheus targets: http://localhost:9091/targets\n")
	return nil
}

func runMonitorMetrics(_ *cobra.Command, _ []string) error {
	metricsPort := getEnvOrDefault("INFRASAGE_METRICS_PORT", "2112")
	metricsURL := fmt.Sprintf("http://localhost:%s/metrics", metricsPort)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(metricsURL)
	if err != nil {
		return fmt.Errorf(
			"cannot reach metrics server at %s: %w\n"+
				"Fix: the metrics server starts when 'infrasage stack up' or any 'infrasage ask' command is run.\n"+
				"     Ensure the binary is running and port %s is not blocked.",
			metricsURL, err, metricsPort,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics server returned HTTP %d — expected 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading metrics response: %w", err)
	}

	os.Stdout.Write(body)
	return nil
}

func runMonitorServe(_ *cobra.Command, _ []string) error {
	metricsPort := getEnvOrDefault("INFRASAGE_METRICS_PORT", "2112")
	fmt.Printf("📡 Keeping persistent metrics endpoint alive on :%s/metrics\n", metricsPort)
	fmt.Println("   Keep this command running while Prometheus/Grafana are scraping.")
	select {}
}
