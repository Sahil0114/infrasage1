package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tf "github.com/Sahil0114/infrasage1/internal/terraform"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply <file>",
	Short: "Run terraform init + validate + plan + apply on a generated .tf file",
	Long: `apply runs the full Terraform workflow against a generated HCL file:
  1. terraform init    — downloads providers
  2. terraform validate — checks HCL syntax
  3. terraform plan    — previews changes
  4. terraform apply   — creates/updates infrastructure

Requires AWS credentials to be configured (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
or an active AWS SSO session).`,
	Args: cobra.ExactArgs(1),
	RunE: runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	tfFile := args[0]

	// Resolve the directory containing the .tf file — terraform commands
	// operate on a directory, not individual files.
	absPath, err := filepath.Abs(tfFile)
	if err != nil {
		return fmt.Errorf("apply: resolving path for %q: %w", tfFile, err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("apply: file %q not found.\nFix: run 'infrasage ask \"...\"' first to generate a Terraform file.", tfFile)
	}

	dir := filepath.Dir(absPath)

	fmt.Printf("🔧 Running Terraform workflow on: %s\n\n", tfFile)

	// Step 1: terraform init
	fmt.Println("── Step 1/4: terraform init ──────────────────────────────")
	if err := tf.Init(dir); err != nil {
		return fmt.Errorf("apply: terraform init failed: %w", err)
	}
	fmt.Println()

	// Step 2: terraform validate
	fmt.Println("── Step 2/4: terraform validate ──────────────────────────")
	if err := tf.Validate(dir); err != nil {
		return fmt.Errorf("apply: terraform validate failed: %w\nFix: inspect the HCL in %s for syntax errors, or re-generate with 'infrasage ask'.", err, tfFile)
	}
	fmt.Println()

	// Step 3: terraform plan
	fmt.Println("── Step 3/4: terraform plan ──────────────────────────────")
	exitCode, err := tf.Plan(dir)
	if err != nil {
		return fmt.Errorf("apply: terraform plan failed: %w", err)
	}
	switch exitCode {
	case 0:
		fmt.Println("\n✅ No changes detected — infrastructure is already up to date.")
		return nil
	case 2:
		fmt.Printf("\n📋 Plan shows changes. Proceeding to apply...\n\n")
	default:
		return fmt.Errorf("apply: terraform plan returned unexpected exit code %d", exitCode)
	}

	// Step 4: terraform apply
	fmt.Println("── Step 4/4: terraform apply ─────────────────────────────")
	if err := tf.Apply(dir); err != nil {
		return fmt.Errorf("apply: terraform apply failed: %w", err)
	}

	fmt.Printf("\n✅ terraform apply complete — infrastructure provisioned from %s\n", tfFile)
	return nil
}
