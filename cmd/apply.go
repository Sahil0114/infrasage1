package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Sahil0114/infrasage1/internal/llm"
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

	workDir, err := prepareApplyWorkspace(absPath)
	if err != nil {
		return fmt.Errorf("apply: preparing isolated workspace: %w", err)
	}

	fmt.Printf("🔧 Running Terraform workflow on: %s\n\n", tfFile)
	fmt.Printf("📁 Isolated apply workspace: %s\n\n", workDir)

	// Step 1: terraform init
	fmt.Println("── Step 1/4: terraform init ──────────────────────────────")
	if err := tf.Init(workDir); err != nil {
		return fmt.Errorf("apply: terraform init failed: %w", err)
	}
	fmt.Println()

	// Step 2: terraform validate
	fmt.Println("── Step 2/4: terraform validate ──────────────────────────")
	if err := tf.Validate(workDir); err != nil {
		fmt.Printf("❌ terraform validate failed: %v\n", err)
		fmt.Println("Attempting to auto-correct using the local LLM...")

		// Read the original HCL
		orig, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return fmt.Errorf("apply: failed to read file for LLM correction: %w", readErr)
		}

		// Build a prompt for the LLM
		prompt := "You are an expert Terraform engineer. Here is a Terraform file and a terraform validate error. Please rewrite the file to fix the error. Only output valid HCL.\n\n" +
			"Terraform file:\n" + string(orig) + "\n\n" +
			"terraform validate error:\n" + err.Error() + "\n"

		client := llm.NewOllamaClient("http://localhost:11434", "qwen2.5-coder:3b")
		response, genErr := client.Generate("You are an expert Terraform engineer. Only output valid HCL.", prompt)
		if genErr != nil {
			return fmt.Errorf("apply: LLM correction failed: %w", genErr)
		}

		candidate := tf.ExtractHCL(response)
		if candidate == "" {
			return fmt.Errorf("apply: LLM did not return valid Terraform HCL.\nFix: retry apply and reject non-HCL output, or run 'infrasage fix %s' first.", tfFile)
		}
		if err := validateApplyHCL(candidate); err != nil {
			return fmt.Errorf("apply: LLM output failed syntax validation: %w\nFix: retry apply and reject suggested prose, or run 'infrasage fix %s'.", err, tfFile)
		}

		// Ask user to accept the fix
		fmt.Println("\nSuggested fix from LLM:\n----------------------")
		fmt.Println(candidate)
		fmt.Println("----------------------")
		fmt.Print("Apply this fix? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) == "y" {
			if writeErr := os.WriteFile(absPath, []byte(candidate+"\n"), 0644); writeErr != nil {
				return fmt.Errorf("apply: failed to write LLM-corrected file: %w", writeErr)
			}
			if writeErr := os.WriteFile(filepath.Join(workDir, "main.tf"), []byte(candidate+"\n"), 0644); writeErr != nil {
				return fmt.Errorf("apply: failed to sync corrected file into workspace: %w", writeErr)
			}
			fmt.Println("✅ LLM fix applied. Re-running terraform validate...")
			if err2 := tf.Validate(workDir); err2 != nil {
				return fmt.Errorf("apply: terraform validate still fails after LLM fix: %w", err2)
			}
			fmt.Println("✅ terraform validate passed after LLM fix.")
		} else {
			fmt.Println("❌ LLM fix not applied. Aborting apply.")
			return fmt.Errorf("apply: terraform validate failed and fix was not applied")
		}
	}
	fmt.Println()

	// Step 3: terraform plan
	fmt.Println("── Step 3/4: terraform plan ──────────────────────────────")
	exitCode, err := tf.Plan(workDir)
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
	if err := tf.Apply(workDir); err != nil {
		return fmt.Errorf("apply: terraform apply failed: %w", err)
	}

	fmt.Printf("\n✅ terraform apply complete — infrastructure provisioned from %s\n", tfFile)
	return nil
}

func prepareApplyWorkspace(sourceFile string) (string, error) {
	baseName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	if baseName == "" {
		baseName = "default"
	}

	workspace := filepath.Join(filepath.Dir(sourceFile), ".infrasage-work", baseName)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return "", fmt.Errorf("create workspace directory: %w", err)
	}

	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return "", fmt.Errorf("read source file: %w", err)
	}

	targetFile := filepath.Join(workspace, "main.tf")
	if err := os.WriteFile(targetFile, content, 0644); err != nil {
		return "", fmt.Errorf("write isolated main.tf: %w", err)
	}

	return workspace, nil
}

func validateApplyHCL(hcl string) error {
	tmpDir, err := os.MkdirTemp("", "infrasage-apply-validate-*")
	if err != nil {
		return fmt.Errorf("create temp validation dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tfPath := filepath.Join(tmpDir, "main.tf")
	if err := os.WriteFile(tfPath, []byte(hcl+"\n"), 0o644); err != nil {
		return fmt.Errorf("write temp validation file: %w", err)
	}

	cmd := exec.Command("terraform", "fmt", "-check")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("terraform fmt -check failed: %v\n%s", err, string(out))
	}

	return nil
}
