package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Sahil0114/infrasage1/internal/gitops"
	"github.com/spf13/cobra"
)

var (
	deployBranch  string
	deployMessage string
)

var deployCmd = &cobra.Command{
	Use:   "deploy <file>",
	Short: "Commit a Terraform file, push a branch, and open a GitHub pull request",
	Long: `deploy commits the given Terraform file to a new git branch, pushes it
to origin, and opens a pull request on GitHub.

Requirements:
  - GITHUB_TOKEN must be set in .env (needs 'repo' scope)
  - GITHUB_REPO must be set in .env (format: owner/repo-name)
  - The current directory must be inside a git repository
  - The file must already be generated (run 'infrasage ask' first)

Example:
  infrasage deploy infra.tf
  infrasage deploy ec2.tf --branch feat/add-ec2 --message "Add EC2 instance"`,
	Args: cobra.ExactArgs(1),
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(
		&deployBranch, "branch", "",
		"Git branch name to create (default: infrasage/<basename-without-ext>-<timestamp>)",
	)
	deployCmd.Flags().StringVar(
		&deployMessage, "message", "",
		"Git commit message (default: derived from filename)",
	)
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	tfFile := args[0]

	// Verify the file exists before doing any git work.
	if _, err := os.Stat(tfFile); err != nil {
		return fmt.Errorf("deploy: file not found: %q\nFix: run 'infrasage ask \"...\"' first to generate the file.", tfFile)
	}

	// Derive branch name if not provided.
	if deployBranch == "" {
		base := filepath.Base(tfFile)
		noExt := base[:len(base)-len(filepath.Ext(base))]
		ts := time.Now().Format("20060102-150405")
		deployBranch = fmt.Sprintf("infrasage/%s-%s", noExt, ts)
	}

	// Derive commit message if not provided.
	if deployMessage == "" {
		deployMessage = fmt.Sprintf("chore(infrasage): add %s", filepath.Base(tfFile))
	}

	fmt.Printf("🚀 Deploying %s\n", tfFile)
	fmt.Printf("   Branch:  %s\n", deployBranch)
	fmt.Printf("   Message: %s\n\n", deployMessage)

	// Step 1: commit and push.
	fmt.Println("📦 Committing and pushing branch...")
	if err := gitops.CommitAndPush(tfFile, deployBranch, deployMessage); err != nil {
		return fmt.Errorf("deploy: git step failed: %w", err)
	}
	fmt.Printf("✅ Branch pushed: %s\n\n", deployBranch)

	// Step 2: open a pull request.
	fmt.Println("🔗 Opening GitHub pull request...")
	prTitle := fmt.Sprintf("[InfraSage] %s", deployMessage)
	prBody := gitops.BuildPRBody(tfFile, deployBranch, "github")

	prURL, err := gitops.CreatePR(deployBranch, prTitle, prBody)
	if err != nil {
		return fmt.Errorf("deploy: creating PR failed: %w", err)
	}

	fmt.Printf("✅ Pull request opened: %s\n", prURL)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Review the PR at the URL above\n")
	fmt.Printf("  2. Merge to trigger Digger's terraform apply\n")
	fmt.Printf("  3. Monitor drift with: infrasage drift\n")

	return nil
}
