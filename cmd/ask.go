package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Sahil0114/infrasage1/internal/llm"
	"github.com/Sahil0114/infrasage1/internal/monitor"
	"github.com/Sahil0114/infrasage1/internal/scanner"
	tf "github.com/Sahil0114/infrasage1/internal/terraform"
	"github.com/spf13/cobra"
)

// systemPrompt is the carefully engineered prompt sent to the LLM with every generation call.
// It is defined here as a constant and must not be moved or user-overridable in Phase 1.
// It is the primary lever for output quality — keep it intact.
const systemPrompt = `You are an expert Terraform engineer. Your only job is to output valid Terraform HCL code.

Rules you must follow without exception:
1. Output ONLY raw HCL code. No markdown. No explanations. No commentary.
2. Start your response with the first line of HCL code.
3. Always include a terraform { required_providers { ... } } block.
4. Always include required_version = ">= 1.6" inside the terraform block.
5. Use the AWS provider unless the user explicitly says otherwise.
6. Follow these security defaults:
   - S3 buckets: block_public_acls = true, block_public_policy = true,
                 ignore_public_acls = true, restrict_public_buckets = true
   - S3 buckets: enable server_side_encryption_configuration
   - S3 buckets: enable versioning
   - Security groups: no ingress 0.0.0.0/0 on port 22 unless asked
   - IAM: no wildcard (*) actions or resources unless asked
7. Add these tags to every resource:
   tags = { Project = "infrasage", ManagedBy = "terraform" }
8. Use snake_case for all resource and variable names.
9. The AWS provider version must be ~> 5.0.
10. Do not use deprecated attributes.`

var (
	askOutFile string
	askNoScan  bool
)

var askCmd = &cobra.Command{
	Use:   `ask "natural language prompt"`,
	Short: "Generate Terraform HCL from a natural language description",
	Long: `Generate production-ready, security-hardened Terraform HCL from a plain English prompt.

The generated HCL is written to a file (default: infra.tf) and automatically
scanned by Checkov, tfsec, and Terrascan unless --no-scan is specified.

Examples:
  infrasage ask "create an S3 bucket with versioning and encryption"
  infrasage ask "create an EC2 instance in a private subnet" --out ec2.tf
  infrasage ask "create an RDS PostgreSQL database" --no-scan`,
	Args: cobra.ExactArgs(1),
	RunE: runAsk,
}

func init() {
	askCmd.Flags().StringVarP(
		&askOutFile, "out", "o", "infra.tf",
		"Output filename for generated HCL",
	)
	askCmd.Flags().BoolVar(
		&askNoScan, "no-scan", false,
		"Skip security scanning after generation",
	)
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	ollamaURL := getEnvOrDefault("INFRASAGE_OLLAMA_URL", "http://localhost:11434")
	ollamaModel := getEnvOrDefault("INFRASAGE_OLLAMA_MODEL", "qwen2.5-coder:3b")

	slog.Debug("ask command starting",
		"url", ollamaURL,
		"model", ollamaModel,
		"out", askOutFile,
		"no_scan", askNoScan,
	)

	// Step 1 — create Ollama client
	client := llm.NewOllamaClient(ollamaURL, ollamaModel)

	// Step 2 — liveness check before paying generation cost
	if !client.IsHealthy() {
		return fmt.Errorf(
			"Ollama is not running at %s.\n"+
				"Fix: run 'ollama serve' in a terminal and wait a few seconds, then retry.\n"+
				"     Or run 'infrasage stack up' which checks and starts Ollama automatically.",
			ollamaURL,
		)
	}

	// Step 3 — user-facing progress indicator
	fmt.Println("⏳ Generating Terraform HCL...")

	// Step 4 — record start time for latency metric
	start := time.Now()

	// Step 5 — call the LLM
	response, err := client.Generate(systemPrompt, args[0])
	elapsed := time.Since(start)

	// Step 6 — handle generation error
	if err != nil {
		monitor.RecordGeneration("error")
		return fmt.Errorf(
			"LLM generation failed after %.1fs: %w\n"+
				"Fix: ensure model '%s' is pulled ('infrasage model pull') and Ollama is running.",
			elapsed.Seconds(), err, ollamaModel,
		)
	}

	slog.Debug("raw LLM response received", "len", len(response), "elapsed", elapsed)

	// Step 7 — strip markdown fences, validate it looks like HCL
	hcl := tf.ExtractHCL(response)

	// Step 8 — bail out if the model returned prose instead of code
	if hcl == "" {
		monitor.RecordGeneration("error")
		return fmt.Errorf(
			"model did not return valid HCL after %.1fs.\n"+
				"Fix: try rephrasing your prompt to be more specific, or run 'infrasage model pull' to refresh the model.",
			elapsed.Seconds(),
		)
	}

	// Step 9 — write HCL to disk
	if err := os.WriteFile(askOutFile, []byte(hcl+"\n"), 0o644); err != nil {
		monitor.RecordGeneration("error")
		return fmt.Errorf("ask: writing output file %q: %w", askOutFile, err)
	}

	// Step 10 — print elapsed time and destination
	fmt.Printf("✅ Generated in %.1fs → %s\n\n", elapsed.Seconds(), askOutFile)

	// Step 11 — print the generated HCL so the user can see it
	fmt.Println(hcl)
	fmt.Println()

	// Step 12 — run security scans unless opted out
	if !askNoScan {
		fmt.Println("🔍 Running security scans (use --no-scan to skip)...")
		report, err := scanner.RunAll(askOutFile)
		if err != nil {
			// Scanner setup error (file path issues etc.) — not a fatal failure
			fmt.Fprintf(os.Stderr, "\n⚠  Scanner error: %v\n", err)
			fmt.Fprintf(os.Stderr, "   The HCL file was still written to %s.\n", askOutFile)
		} else {
			report.Print()
			monitor.RecordScan(report)
			// Save Checkov JSON output for fix command
			for _, res := range report.Results {
				if res.Tool == "checkov" && res.RawJSON != nil {
					jsonFile := askOutFile + ".checkov.json"
					if err := os.WriteFile(jsonFile, res.RawJSON, 0644); err == nil {
						fmt.Printf("ℹ️  Checkov JSON output saved to %s\n", jsonFile)
					}
				}
			}
		}
	}

	// Step 13 — record success metrics
	monitor.RecordGeneration("success")
	monitor.RecordModelLatency(elapsed.Seconds())

	return nil
}

// getEnvOrDefault returns the value of the environment variable key,
// or defaultVal if the variable is unset or empty.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
