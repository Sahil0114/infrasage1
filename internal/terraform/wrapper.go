package terraform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Init runs terraform init in the given directory.
func Init(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "init")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("terraform init timed out after 120s")
		}
		return fmt.Errorf("terraform init: %w\nFix: ensure terraform is installed ('brew install terraform') and you have network access.", err)
	}
	return nil
}

// InitWithOutput runs terraform init and returns its combined output.
func InitWithOutput(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "init", "-input=false")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return string(output), fmt.Errorf("terraform init timed out after 120s")
		}
		return string(output), fmt.Errorf("terraform init: %w\nFix: ensure terraform is installed ('brew install terraform') and you have network access.", err)
	}
	return string(output), nil
}

// Validate runs terraform validate in the given directory.
// It assumes terraform init has already been run.
func Validate(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "validate")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("terraform validate timed out after 60s")
		}
		return fmt.Errorf("terraform validate failed: %w\nFix: check the generated HCL for syntax errors.", err)
	}
	return nil
}

// Plan runs terraform plan with --detailed-exitcode in the given directory.
// Return values:
//
//	exitCode 0 = no changes
//	exitCode 2 = changes present (not an error — this is drift or a pending apply)
//	error      = terraform itself failed (syntax error, provider auth, etc.)
func Plan(dir string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "plan", "-detailed-exitcode")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, fmt.Errorf("terraform plan timed out after 300s")
		}
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			code := exitErr.ExitCode()
			if code == 2 {
				// Exit code 2 means changes are present — not an error.
				return 2, nil
			}
			// Any other non-zero code is a real failure.
			return code, fmt.Errorf("terraform plan exited with code %d\nFix: check provider credentials and resource configuration.", code)
		}
		return -1, fmt.Errorf("terraform plan: %w", err)
	}
	return 0, nil
}

// PlanWithOutput runs terraform plan -detailed-exitcode and returns exit code + output.
func PlanWithOutput(dir string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "plan", "-no-color", "-input=false", "-detailed-exitcode")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, string(output), fmt.Errorf("terraform plan timed out after 300s")
		}
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			code := exitErr.ExitCode()
			if code == 2 {
				return 2, string(output), nil
			}
			return code, string(output), fmt.Errorf("terraform plan exited with code %d\nFix: check provider credentials and resource configuration.", code)
		}
		return -1, string(output), fmt.Errorf("terraform plan: %w", err)
	}

	return 0, string(output), nil
}

// Apply runs terraform apply -auto-approve in the given directory.
// It assumes terraform init has already been run.
func Apply(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "terraform", "apply", "-auto-approve")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("terraform apply timed out after 600s")
		}
		return fmt.Errorf("terraform apply failed: %w\nFix: check provider credentials and resource limits.", err)
	}
	return nil
}
