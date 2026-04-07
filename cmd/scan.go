package cmd

import (
	"fmt"
	"os"

	"github.com/Sahil0114/infrasage1/internal/monitor"
	"github.com/Sahil0114/infrasage1/internal/scanner"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan <file>",
	Short: "Run all three security scanners against a Terraform file",
	Long: `Runs Checkov, tfsec, and Terrascan in parallel against the given Terraform file.
Results are printed as a unified table. Exit code 1 if any CRITICAL findings are found.

The scanner containers must be running first:
  infrasage stack up`,
	Args: cobra.ExactArgs(1),
	RunE: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	tfFile := args[0]

	// Ensure the file exists before attempting to scan it.
	if _, err := os.Stat(tfFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %q\nFix: provide the path to a valid Terraform (.tf) file.", tfFile)
	}

	fmt.Printf("🔍 Scanning %s with Checkov, tfsec, and Terrascan...\n\n", tfFile)

	report, err := scanner.RunAll(tfFile)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	report.Print()

	// Record metrics for each finding.
	for _, res := range report.Results {
		if res.Error == nil {
			for _, f := range res.Findings {
				monitor.RecordScanFinding(res.Tool, f.Severity)
			}
		}
	}

	// Exit with code 1 if any CRITICAL findings were found, so this command
	// can be used in CI pipelines that gate on exit code.
	if report.HasCritical() {
		fmt.Fprintf(os.Stderr, "❌ CRITICAL findings detected. Review the report above before deploying.\n")
		os.Exit(1)
	}

	return nil
}
