package cmd

import (
	"fmt"

	"github.com/Sahil0114/infrasage1/internal/gitops"
	"github.com/spf13/cobra"
)

var configAWS bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure InfraSage integrations",
	Long: `Configure integrations such as AWS deployment mode.
Example:
  infrasage config --aws`,
	RunE: runConfig,
}

func init() {
	configCmd.Flags().BoolVar(&configAWS, "aws", false, "Sync AWS credentials to GitHub Actions secrets")
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	if !configAWS {
		return fmt.Errorf("no configuration target specified\nFix: run 'infrasage config --aws' to sync AWS secrets")
	}

	creds, ok, err := gitops.DetectAWSCredentials()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("AWS credentials not found\nFix: run 'aws configure' or export AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY")
	}

	if err := gitops.SyncAWSSecrets(creds); err != nil {
		return err
	}

	fmt.Println("✅ AWS credentials synced to GitHub Actions secrets.")
	return nil
}
