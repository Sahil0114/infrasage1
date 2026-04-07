package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tf "github.com/Sahil0114/infrasage1/internal/terraform"
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect infrastructure drift by running terraform plan",
	Long: `Runs 'terraform plan --detailed-exitcode' in the current directory.

Exit codes from terraform plan:
  0 = No changes. Infrastructure matches configuration. No drift.
  2 = Changes detected. Drift or pending apply exists.
  1 = Error (authentication, syntax, etc.)

InfraSage translates these into a human-readable status message.`,
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

	// Run terraform init first so providers are available.
	fmt.Println("⚙️  Running terraform init...")
	if err := tf.Init(dir); err != nil {
		return fmt.Errorf("drift: terraform init failed: %w\nFix: ensure you have network access and valid provider configuration.", err)
	}

	fmt.Println("\n⚙️  Running terraform plan (drift check)...")
	exitCode, err := tf.Plan(dir)
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
		// Return a non-nil error with exit code 2 semantics so the shell can detect drift.
		// We use a plain error message so the cobra layer prints it cleanly.
		os.Exit(2)
	default:
		return fmt.Errorf("drift: terraform plan returned unexpected exit code %d", exitCode)
	}

	return nil
}
