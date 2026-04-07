package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var debug bool

// rootCmd is the base command. Every subcommand registers itself to this via init().
var rootCmd = &cobra.Command{
	Use:     "infrasage",
	Short:   "AI-powered DevSecOps CLI — generate, scan and deploy Terraform with a single command",
	Version: "0.1.0",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if debug {
			slog.SetDefault(slog.New(
				slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}),
			))
			slog.Debug("debug logging enabled")
		}
		return nil
	},
}

func init() {
	// Load .env file if it exists. Silently ignore if not found — all vars have
	// safe defaults and the binary works without a .env file.
	if err := godotenv.Load(); err != nil {
		slog.Debug("no .env file found, using environment variables and defaults")
	}

	rootCmd.PersistentFlags().BoolVar(
		&debug, "debug", false,
		"Enable debug-level logging to stderr",
	)
}

// Execute is the single entry point called from main.go.
// It runs the root command and exits the process on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
