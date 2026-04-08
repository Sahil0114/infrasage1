package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sahil0114/infrasage1/internal/monitor"
	tf "github.com/Sahil0114/infrasage1/internal/terraform"
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift [file]",
	Short: "Detect infrastructure drift by running terraform plan",
	Long: `Runs 'terraform plan --detailed-exitcode' in an isolated workspace.

Exit codes from terraform plan:
  0 = No changes. Infrastructure matches configuration. No drift.
  2 = Changes detected. Drift or pending apply exists.
  1 = Error (authentication, syntax, etc.)

InfraSage translates these into a human-readable status message.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDrift,
}

func init() {
	rootCmd.AddCommand(driftCmd)
}

func runDrift(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("drift: resolving working directory: %w", err)
	}

	// Verify terraform is available.
	fmt.Println("🔍 Checking for infrastructure drift...")
	fmt.Printf("   Directory: %s\n\n", dir)

	// Ensure the directory contains at least one .tf file.
	matches, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("drift: no .tf files found in %s\nFix: run 'infrasage ask \"...\"' first to generate a Terraform file, or cd into a directory that contains .tf files.", dir)
	}

	var targetFile string
	if len(args) == 1 {
		targetFile, err = filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("drift: resolving path for %q: %w", args[0], err)
		}
		if _, err := os.Stat(targetFile); err != nil {
			return fmt.Errorf("drift: file %q not found.\nFix: provide a valid Terraform file path.", args[0])
		}
	} else {
		if len(matches) > 1 {
			return fmt.Errorf("drift: multiple Terraform files found in %s.\nFix: run 'infrasage drift <file>' to target one file and avoid duplicate provider/resource conflicts.", dir)
		}
		targetFile = matches[0]
	}

	workDir, err := prepareDriftWorkspace(targetFile)
	if err != nil {
		return fmt.Errorf("drift: preparing isolated workspace: %w", err)
	}
	fmt.Printf("   File: %s\n", filepath.Base(targetFile))
	fmt.Printf("   Isolated workspace: %s\n\n", workDir)

	// Run terraform init first so providers are available.
	fmt.Println("⚙️  Running terraform init...")
	if err := tf.Init(workDir); err != nil {
		return fmt.Errorf("drift: terraform init failed: %w\nFix: ensure you have network access and valid provider configuration.", err)
	}

	fmt.Println("\n⚙️  Running terraform plan (drift check)...")
	exitCode, err := tf.Plan(workDir)
	if err != nil {
		return fmt.Errorf("drift: terraform plan failed: %w", err)
	}

	fmt.Println()
	switch exitCode {
	case 0:
		fmt.Println("✅ No drift detected — infrastructure matches the Terraform configuration.")
	case 2:
		fmt.Fprintln(os.Stderr, "⚠️  Drift detected — the live infrastructure differs from the Terraform configuration.")
		fmt.Fprintln(os.Stderr, "   Review the plan output above, then run 'infrasage apply <file>' to reconcile.")
		monitor.RecordDrift("terraform")
		// Return a non-nil error with exit code 2 semantics so the shell can detect drift.
		// We use a plain error message so the cobra layer prints it cleanly.
		os.Exit(2)
	default:
		return fmt.Errorf("drift: terraform plan returned unexpected exit code %d", exitCode)
	}

	return nil
}

func prepareDriftWorkspace(sourceFile string) (string, error) {
	baseName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	if baseName == "" {
		baseName = "default"
	}

	workspace := filepath.Join(filepath.Dir(sourceFile), ".infrasage-work", baseName+"-drift")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", fmt.Errorf("create workspace directory: %w", err)
	}

	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return "", fmt.Errorf("read source file: %w", err)
	}

	targetFile := filepath.Join(workspace, "main.tf")
	if err := os.WriteFile(targetFile, content, 0o644); err != nil {
		return "", fmt.Errorf("write isolated main.tf: %w", err)
	}

	return workspace, nil
}
