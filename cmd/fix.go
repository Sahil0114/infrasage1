package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Sahil0114/infrasage1/internal/llm"
	tf "github.com/Sahil0114/infrasage1/internal/terraform"
	"github.com/spf13/cobra"
)

var (
	fixDryRun  bool
	fixOutFile string
)

var fixCmd = &cobra.Command{
	Use:   "fix <file>",
	Short: "Auto-remediate Checkov issues in a Terraform file using the local LLM",
	Long:  `Runs Checkov on the given Terraform file, parses failed checks, and prompts the local LLM to rewrite the file to fix each issue. Writes the improved HCL to <file>.fixed.tf by default, or to --out if specified. Use --dry-run to print the fix without writing.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runFix,
}

func init() {
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Print the fixed HCL instead of writing to a file")
	fixCmd.Flags().StringVar(&fixOutFile, "out", "", "Output file for the fixed HCL (default: <file>.fixed.tf)")
	rootCmd.AddCommand(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) error {
	file := args[0]
	outFile := fixOutFile
	if outFile == "" {
		outFile = file + ".fixed.tf"
	}

	// Try to find existing Checkov JSON output
	checkovJSON := file + ".checkov.json"
	var output []byte
	var result struct {
		Results struct {
			FailedChecks []struct {
				CheckID         string          `json:"check_id"`
				Resource        string          `json:"resource"`
				FileLineRange   []int           `json:"file_line_range"`
				Guideline       string          `json:"guideline"`
				CheckName       string          `json:"check_name"`
				Details         interface{}     `json:"details"`
				CodeBlock       [][]interface{} `json:"code_block"`
				SuppressComment string          `json:"suppress_comment"`
				Severity        string          `json:"severity"`
				Description     string          `json:"description"`
			} `json:"failed_checks"`
		} `json:"results"`
	}
	if data, err := os.ReadFile(checkovJSON); err == nil {
		output = data
		if err := json.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("fix: failed to parse existing Checkov JSON: %w", err)
		}
		fmt.Printf("ℹ️  Using existing Checkov output: %s\n", checkovJSON)
	} else {
		// Fallback: run Checkov locally if available
		checkovPath, err := exec.LookPath("checkov")
		if err != nil {
			return fmt.Errorf("fix: no Checkov JSON found and checkov not installed locally.\nFix: install Checkov with 'pip install checkov' or run 'infrasage ask' to generate scan output.")
		}
		checkovCmd := exec.Command(checkovPath, "-f", file, "--output", "json")
		checkovCmd.Dir = "."
		output, err = checkovCmd.Output()
		if err != nil {
			return fmt.Errorf("fix: failed to run local Checkov: %w\nFix: ensure checkov is installed and the file is valid.", err)
		}
		if err := json.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("fix: failed to parse Checkov JSON: %w", err)
		}
		fmt.Println("ℹ️  Ran local Checkov binary.")
	}

	if len(result.Results.FailedChecks) == 0 {
		fmt.Println("✅ No Checkov issues found!")
		return nil
	}

	// Read the original HCL
	orig, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("fix: failed to read file: %w", err)
	}

	// Build a prompt for the LLM
	var sb strings.Builder
	sb.WriteString("You are an expert Terraform engineer. Here is a Terraform file and a list of Checkov findings. Please rewrite the file to fix all issues, following best practices.\n\n")
	sb.WriteString("Terraform file:\n" + string(orig) + "\n\n")
	sb.WriteString("Checkov findings:\n")
	for _, fc := range result.Results.FailedChecks {
		// Handle details as string or array
		detailsStr := ""
		switch v := fc.Details.(type) {
		case string:
			detailsStr = v
		case []interface{}:
			var parts []string
			for _, item := range v {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			detailsStr = strings.Join(parts, "; ")
		}
		sb.WriteString("- [" + fc.CheckID + "] " + fc.CheckName + ": " + fc.Description)
		if detailsStr != "" {
			sb.WriteString(" Details: " + detailsStr)
		}
		sb.WriteString("\n")
	}

	client := llm.NewOllamaClient("http://localhost:11434", "qwen2.5-coder:3b")
	systemPrompt := "You are an expert Terraform engineer. Your only job is to output valid Terraform HCL code. Output ONLY raw HCL code. No markdown. No explanations. No commentary. Start your response with the first line of HCL code."
	prompt := sb.String()

	hcl, err := generateAndValidateFix(client, systemPrompt, prompt)
	if err != nil {
		return err
	}

	if fixDryRun {
		fmt.Println("\n--- Fixed HCL (dry run) ---")
		fmt.Println(hcl)
		fmt.Println("--- End ---")
		return nil
	}

	// Confirm before overwriting existing file
	if outFile == file {
		fmt.Printf("You are about to overwrite %s. Continue? [y/N]: ", file)
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("❌ Fix not applied. Aborting.")
			return nil
		}
	}

	if err := os.WriteFile(outFile, []byte(hcl), 0644); err != nil {
		return fmt.Errorf("fix: failed to write output: %w", err)
	}

	fmt.Printf("✨ Fixed file written to %s\n", outFile)
	return nil
}

func generateAndValidateFix(client *llm.OllamaClient, systemPrompt, basePrompt string) (string, error) {
	response, err := client.Generate(systemPrompt, basePrompt)
	if err != nil {
		return "", fmt.Errorf("fix: LLM generation failed: %w", err)
	}

	hcl := tf.ExtractHCL(response)
	if hcl == "" {
		return "", fmt.Errorf("fix: LLM did not return valid HCL.\nRaw response:\n%s", response)
	}

	if err := validateGeneratedHCL(hcl); err == nil {
		return hcl, nil
	}

	repairPrompt := basePrompt + "\n\nYour previous output was invalid Terraform syntax." +
		" Return corrected Terraform HCL only.\n\nPrevious output:\n" + response

	retryResp, retryErr := client.Generate(systemPrompt, repairPrompt)
	if retryErr != nil {
		return "", fmt.Errorf("fix: LLM retry failed after syntax validation error: %w", retryErr)
	}

	retryHCL := tf.ExtractHCL(retryResp)
	if retryHCL == "" {
		return "", fmt.Errorf("fix: retry output still did not contain valid HCL.\nRaw response:\n%s", retryResp)
	}

	if err := validateGeneratedHCL(retryHCL); err != nil {
		return "", fmt.Errorf("fix: generated HCL remains invalid after retry: %w", err)
	}

	fmt.Println("ℹ️  Initial LLM fix had syntax issues; retry produced valid HCL.")
	return retryHCL, nil
}

func validateGeneratedHCL(hcl string) error {
	if tf.ExtractHCL(hcl) == "" {
		return fmt.Errorf("output is not recognizable Terraform HCL")
	}

	if _, err := exec.LookPath("terraform"); err != nil {
		return fmt.Errorf("terraform binary not found in PATH\nFix: run 'brew install terraform' to validate generated fixes")
	}

	tmpDir, err := os.MkdirTemp("", "infrasage-fix-validate-*")
	if err != nil {
		return fmt.Errorf("create temp validation dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tfPath := filepath.Join(tmpDir, "main.tf")
	if err := os.WriteFile(tfPath, []byte(hcl+"\n"), 0o644); err != nil {
		return fmt.Errorf("write temp validation file: %w", err)
	}

	// terraform fmt parses HCL and fails fast on syntax errors without provider downloads.
	cmd := exec.Command("terraform", "fmt", "-check")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("terraform syntax validation failed: %v\n%s", err, string(out))
	}

	return nil
}
