package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// AWSCredentials holds AWS credentials resolved from the environment or AWS CLI.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Profile         string
}

// DetectAWSCredentials checks for AWS credentials via env vars or aws configure.
// It returns (creds, true, nil) when valid credentials are found.
func DetectAWSCredentials() (AWSCredentials, bool, error) {
	creds := AWSCredentials{
		AccessKeyID:     strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")),
		SessionToken:    strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN")),
		Region:          strings.TrimSpace(os.Getenv("AWS_REGION")),
		Profile:         strings.TrimSpace(os.Getenv("AWS_PROFILE")),
	}

	if creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		if creds.Region == "" {
			creds.Region = "us-east-1"
		}
		return creds, true, nil
	}

	if _, err := exec.LookPath("aws"); err != nil {
		return creds, false, nil
	}

	if _, err := awsCommandOutput(creds.Profile, "configure", "list"); err != nil {
		return creds, false, fmt.Errorf("aws configure list failed: %w\nFix: run 'aws configure' to set credentials.", err)
	}

	accessKey, err := awsCommandOutput(creds.Profile, "configure", "get", "aws_access_key_id")
	if err != nil {
		return creds, false, fmt.Errorf("aws configure get aws_access_key_id failed: %w\nFix: run 'aws configure' to set credentials.", err)
	}
	secretKey, err := awsCommandOutput(creds.Profile, "configure", "get", "aws_secret_access_key")
	if err != nil {
		return creds, false, fmt.Errorf("aws configure get aws_secret_access_key failed: %w\nFix: run 'aws configure' to set credentials.", err)
	}
	region, err := awsCommandOutput(creds.Profile, "configure", "get", "region")
	if err != nil {
		return creds, false, fmt.Errorf("aws configure get region failed: %w\nFix: run 'aws configure' to set credentials.", err)
	}

	creds.AccessKeyID = strings.TrimSpace(accessKey)
	creds.SecretAccessKey = strings.TrimSpace(secretKey)
	creds.Region = strings.TrimSpace(region)

	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return creds, false, nil
	}
	if creds.Region == "" {
		creds.Region = "us-east-1"
	}

	slog.Debug("aws credentials detected", "profile", creds.Profile, "region", creds.Region)
	return creds, true, nil
}

// SyncAWSSecrets pushes AWS credentials into GitHub Actions secrets via gh CLI.
func SyncAWSSecrets(creds AWSCredentials) error {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return fmt.Errorf("missing AWS credentials\nFix: run 'aws configure' or export AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY.")
	}

	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		return fmt.Errorf("GITHUB_REPO environment variable is not set.\nFix: add GITHUB_REPO=owner/repo-name to your .env file.")
	}

	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) not found in PATH.\nFix: install from https://cli.github.com and run 'gh auth login'.")
	}

	if err := setGitHubSecret(repo, "AWS_ACCESS_KEY_ID", creds.AccessKeyID); err != nil {
		return err
	}
	if err := setGitHubSecret(repo, "AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey); err != nil {
		return err
	}
	if creds.SessionToken != "" {
		if err := setGitHubSecret(repo, "AWS_SESSION_TOKEN", creds.SessionToken); err != nil {
			return err
		}
	}
	if creds.Region != "" {
		if err := setGitHubSecret(repo, "AWS_REGION", creds.Region); err != nil {
			return err
		}
	}

	slog.Info("aws secrets synced to GitHub", "repo", repo)
	return nil
}

func setGitHubSecret(repo, name, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "secret", "set", name, "--repo", repo, "--app", "actions")
	cmd.Stdin = strings.NewReader(value)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("gh secret set %s timed out after 20s", name)
		}
		return fmt.Errorf("gh secret set %s failed: %w\nFix: ensure 'gh auth login' has been completed and you have repo admin access.", name, err)
	}
	return nil
}

func awsCommandOutput(profile string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("aws %s timed out after 10s", strings.Join(args, " "))
		}
		return "", err
	}
	return string(out), nil
}
